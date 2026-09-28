package homereset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

var storePath = regexp.MustCompile(`^/nix/store/[0-9abcdfghijklmnpqrsvwxyz]{32}-[A-Za-z0-9+._?=-]+(?:/[^\x00-\x20'\\]+)*$`)

type seedManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Profile       json.RawMessage `json:"profile"`
	Extensions    []seedExtension `json:"extensions"`
}

type seedExtension struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Source  string `json:"source"`
}

func decodeStrict(data []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func readBounded(name string, limit int64) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds the supported size")
	}
	return data, nil
}

func immutableStoreFile(name string) error {
	if !storePath.MatchString(name) || filepath.Clean(name) != name {
		return errors.New("expected a canonical immutable store path")
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		return err
	}
	if !storePath.MatchString(resolved) {
		return errors.New("store reference escapes the store")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Uid != 0 || info.Mode().Perm()&0222 != 0 {
		return errors.New("store file must be immutable and root-owned")
	}
	return nil
}

// validateSeed inspects the entire small preference/scaffold payload before
// mutation. Only declared, identity-checked extension links may leave the seed.
// The Nix store is a trusted input, never a user-writable home template.
func validateSeed(seed string) error {
	if !storePath.MatchString(seed) || filepath.Dir(seed) != "/nix/store" {
		return errors.New("seed must be a direct store output")
	}
	for _, name := range []string{seed, seed + "/home", seed + "/dconf"} {
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Sys().(*syscall.Stat_t).Uid != 0 || info.Mode().Perm()&0222 != 0 {
			return errors.New("seed directory is not immutable")
		}
	}
	for _, name := range []string{"manifest.json", "dconf/00-workspace"} {
		if err := immutableStoreFile(seed + "/" + name); err != nil {
			return err
		}
	}
	data, err := readBounded(seed+"/manifest.json", 128*1024)
	if err != nil {
		return err
	}
	var manifest seedManifest
	if err := decodeStrict(data, &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 {
		return errors.New("unsupported seed manifest")
	}
	profile, issues := domain.DecodeWorkspaceProfile(manifest.Profile)
	if len(issues) != 0 {
		return errors.New("invalid seed profile")
	}
	var wanted []string
	if profile.VSCode != nil && profile.VSCode.Extensions != nil {
		wanted = *profile.VSCode.Extensions
	}
	var ids []string
	links := make(map[string]string)
	for _, extension := range manifest.Extensions {
		if extension.Path != ".vscode/extensions/"+extension.ID || extension.Version == "" || !storePath.MatchString(extension.Source) || filepath.Clean(extension.Source) != extension.Source {
			return errors.New("invalid extension manifest entry")
		}
		if !strings.HasSuffix(extension.Source, "/share/vscode/extensions/"+extension.ID) {
			return errors.New("unexpected extension payload path")
		}
		if _, exists := links[extension.Path]; exists {
			return errors.New("duplicate extension manifest entry")
		}
		if err := immutableStoreFile(extension.Source + "/package.json"); err != nil {
			return err
		}
		payload, err := readBounded(extension.Source+"/package.json", 4*1024*1024)
		if err != nil {
			return err
		}
		var identity struct {
			Publisher string
			Name      string
			Version   string
		}
		if err := json.Unmarshal(payload, &identity); err != nil {
			return err
		}
		if strings.ToLower(identity.Publisher+"."+identity.Name) != extension.ID || identity.Version != extension.Version {
			return errors.New("extension payload identity mismatch")
		}
		ids = append(ids, extension.ID)
		links[extension.Path] = extension.Source
	}
	if !slices.Equal(ids, wanted) {
		return errors.New("seed extensions differ from the profile")
	}
	count, size := 0, int64(0)
	err = filepath.WalkDir(seed+"/home", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 512 {
			return errors.New("seed has too many entries")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Sys().(*syscall.Stat_t).Uid != 0 {
			return errors.New("seed entry is not root-owned")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			relative, err := filepath.Rel(seed+"/home", name)
			if err != nil {
				return err
			}
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			if links[relative] == "" || links[relative] != target {
				return errors.New("undeclared seed link")
			}
			delete(links, relative)
			return nil
		}
		if info.Mode().Perm()&0222 != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("unsupported or writable seed entry")
		}
		size += info.Size()
		if size > 16*1024*1024 {
			return errors.New("seed exceeds the supported size")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(links) != 0 {
		return errors.New("declared extension link is missing")
	}
	return nil
}

// copyTree reads only a previously checked immutable/private tree. The caller
// must own both roots exclusively; never use this for a live student's tree.
func copyTree(source, destination string, uid, gid int) error {
	return filepath.WalkDir(source, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		switch {
		case entry.IsDir():
			if relative != "." {
				if err := os.Mkdir(target, 0700); err != nil {
					return err
				}
			}
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(name)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		case entry.Type().IsRegular():
			if err := copyFile(name, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported prepared entry: %q", relative)
		}
		return os.Lchown(target, uid, gid)
	})
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}
