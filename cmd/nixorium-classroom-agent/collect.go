package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"golang.org/x/sys/unix"
)

// collector hands the files of this session's desktop to the controller,
// which asks for them by their position in the list it received. It only
// reads: nothing on the desktop is changed or removed. Files are opened
// below the desktop folder that was listed, never through a link, and must
// still be the regular file of the listed size.
type collector struct {
	desktop *os.File
	entries []classroomview.FileEntry
	// index is the file being read; sent counts its bytes handed over.
	index int
	file  *os.File
	sent  int64
}

func newCollector(home string) (*collector, int, error) {
	desktop, err := classroomview.DesktopDirectory(home)
	if err != nil {
		return nil, 0, err
	}
	outgoing := &collector{index: -1}
	// The folder is held open first, so the files read later are those
	// below the folder that is listed now.
	fd, err := unix.Open(desktop, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	switch {
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENOTDIR), errors.Is(err, unix.ELOOP):
		outgoing.entries = []classroomview.FileEntry{}
		return outgoing, 0, nil
	case err != nil:
		return nil, 0, err
	}
	outgoing.desktop = os.NewFile(uintptr(fd), desktop)
	entries, skipped, err := classroomview.ListDesktop(desktop)
	if err != nil {
		outgoing.close()
		return nil, skipped, err
	}
	var held, listed unix.Stat_t
	if unix.Fstat(fd, &held) != nil || unix.Lstat(desktop, &listed) != nil || held.Dev != listed.Dev || held.Ino != listed.Ino {
		outgoing.close()
		return nil, skipped, errors.New("the desktop folder changed while it was listed")
	}
	outgoing.entries = entries
	return outgoing, skipped, nil
}

// piece returns the next piece of the file at index; a request for a
// later file ends the current one.
func (outgoing *collector) piece(index int) ([]byte, error) {
	if index < 0 || index >= len(outgoing.entries) || outgoing.entries[index].Dir || outgoing.entries[index].Size == 0 || index < outgoing.index {
		return nil, errors.New("no such file to collect")
	}
	entry := outgoing.entries[index]
	if index != outgoing.index || outgoing.file == nil {
		if index == outgoing.index {
			return nil, errors.New("this file was already collected")
		}
		outgoing.closeFile()
		file, err := outgoing.open(entry)
		if err != nil {
			return nil, err
		}
		outgoing.index, outgoing.file, outgoing.sent = index, file, 0
	}
	data := make([]byte, min(int64(classroomview.MaxChunkBytes), entry.Size-outgoing.sent))
	count, err := io.ReadFull(outgoing.file, data)
	if err != nil || count != len(data) {
		outgoing.closeFile()
		return nil, errors.New("a file changed while it was collected")
	}
	outgoing.sent += int64(count)
	if outgoing.sent == entry.Size {
		outgoing.closeFile()
	}
	return data, nil
}

// open opens a listed file read-only below the desktop folder without
// following links; it must still be a regular file of the listed size.
func (outgoing *collector) open(entry classroomview.FileEntry) (*os.File, error) {
	if outgoing.desktop == nil {
		return nil, errors.New("no such file to collect")
	}
	fd, err := unix.Openat2(int(outgoing.desktop.Fd()), entry.Path, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, errors.New("a file changed while it was collected")
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Size != entry.Size {
		_ = unix.Close(fd)
		return nil, errors.New("a file changed while it was collected")
	}
	return os.NewFile(uintptr(fd), entry.Path), nil
}

func (outgoing *collector) closeFile() {
	if outgoing.file != nil {
		_ = outgoing.file.Close()
		outgoing.file = nil
	}
}

func (outgoing *collector) close() {
	outgoing.closeFile()
	if outgoing.desktop != nil {
		_ = outgoing.desktop.Close()
		outgoing.desktop = nil
	}
}

// collectReply advances a collection of this desktop; any refusal ends it.
func collectReply(outgoing **collector, home string, message classroomview.Message) classroomview.Message {
	refuse := func(code string, err error) classroomview.Message {
		if *outgoing != nil {
			(*outgoing).close()
			*outgoing = nil
		}
		return classroomview.Message{Type: classroomview.TypeError, Code: code, Detail: err.Error()}
	}
	switch message.Type {
	case classroomview.TypeCollectList:
		if *outgoing != nil {
			(*outgoing).close()
			*outgoing = nil
		}
		started, skipped, err := newCollector(home)
		if errors.Is(err, classroomview.ErrCollectTooMuch) {
			return refuse(classroomview.CodeCollectTooMuch, err)
		}
		if err != nil {
			return refuse(classroomview.CodeCollectRefused, err)
		}
		*outgoing = started
		return classroomview.Message{Type: classroomview.TypeCollectListed, Entries: started.entries, Skipped: skipped}
	case classroomview.TypeCollectChunk:
		if *outgoing == nil {
			return refuse(classroomview.CodeCollectRefused, errors.New("no collection is open"))
		}
		data, err := (*outgoing).piece(message.Index)
		if err != nil {
			return refuse(classroomview.CodeCollectRefused, err)
		}
		return classroomview.Message{Type: classroomview.TypeCollectData, Index: message.Index, Data: data}
	default:
		if *outgoing != nil {
			(*outgoing).close()
			*outgoing = nil
		}
		return classroomview.Message{Type: classroomview.TypeCollectDone}
	}
}

// probeCollect collects the session's desktop through a command, as the
// controller does, and saves it into folder: a check of collecting.
func probeCollect(command []string, folder string, output io.Writer) error {
	if len(command) == 0 {
		return errors.New("probe needs a command, for example nixorium-classroom-connect")
	}
	process := exec.Command(command[0], command[1:]...)
	stdin, err := process.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return err
	}
	process.Stderr = os.Stderr
	if err := process.Start(); err != nil {
		return err
	}
	defer func() {
		_ = stdin.Close()
		_ = process.Wait()
	}()
	exchange := func(message classroomview.Message, want string) (classroomview.Message, error) {
		if err := classroomview.Write(stdin, message); err != nil {
			return classroomview.Message{}, err
		}
		reply, err := classroomview.Read(stdout)
		if err == nil && reply.Type != want {
			err = fmt.Errorf("the agent answered %s %s: %s", reply.Type, reply.Code, reply.Detail)
		}
		return reply, err
	}
	if _, err := exchange(classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}, classroomview.TypeHello); err != nil {
		return err
	}
	listed, err := exchange(classroomview.Message{Type: classroomview.TypeCollectList}, classroomview.TypeCollectListed)
	if err != nil {
		return err
	}
	if err := classroomview.ValidateCollectEntries(listed.Entries); err != nil {
		return err
	}
	if len(listed.Entries) == 0 {
		fmt.Fprintln(output, "collect.done nothing to collect")
		return nil
	}
	writer, err := classroomview.NewTreeWriter(folder, listed.Entries)
	if err != nil {
		return err
	}
	for index, entry := range listed.Entries {
		for received := int64(0); !entry.Dir && received < entry.Size; {
			reply, err := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: index}, classroomview.TypeCollectData)
			if err == nil && reply.Index != index {
				err = errors.New("the agent sent a piece of another file")
			}
			if err == nil {
				err = writer.Write(index, reply.Data)
			}
			if err != nil {
				writer.Abort()
				return err
			}
			received += int64(len(reply.Data))
		}
	}
	placed, err := writer.Finish()
	if err != nil {
		return err
	}
	if _, err := exchange(classroomview.Message{Type: classroomview.TypeCollectEnd}, classroomview.TypeCollectDone); err != nil {
		return err
	}
	fmt.Fprintf(output, "collect.done %s skipped=%d\n", strings.Join(placed, ", "), listed.Skipped)
	return nil
}
