package adapters

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// Import bytes stay local to the controller tools. Remember their Git object
// identities across the complete baseline ancestry so renaming or copying a
// blob cannot turn an imported payload into model-visible configuration.
// Excessive or unavailable history fails closed; it never silently exposes it.
func assistantImportedBlobs(ctx context.Context, repository, base string) (map[string]bool, error) {
	if err := assistantImportCompleteHistory(ctx, repository); err != nil {
		return nil, err
	}
	data, err := assistantHistoryGit(ctx, repository, nil, 8*1024*1024,
		"--literal-pathspecs", "log", "--format=", "--raw", "-z", "--no-abbrev", "--no-renames", "--root", "--full-history", "-m", base, "--", "home-files/imports")
	if err != nil {
		return nil, err
	}
	denied := map[string]bool{}
	for _, match := range assistantImportRawObjects.FindAllStringSubmatch(data, -1) {
		for _, object := range match[1:] {
			if strings.Trim(object, "0") != "" {
				denied[object] = true
			}
		}
	}
	return denied, nil
}

var assistantImportRawObjects = regexp.MustCompile(`(?:^|[\n\x00]):[0-7]{6} [0-7]{6} ([0-9a-f]{40}|[0-9a-f]{64}) ([0-9a-f]{40}|[0-9a-f]{64}) [A-Z][0-9]*\x00`)

func assistantDeniedBlob(data []byte, denied map[string]bool) bool {
	if len(denied) == 0 {
		return false
	}
	header := fmt.Sprintf("blob %d\x00", len(data))
	// These are Git object identities, not security or integrity assurances.
	sha1Object := sha1.New()
	sha1Object.Write([]byte(header))
	sha1Object.Write(data)
	sha256Object := sha256.New()
	sha256Object.Write([]byte(header))
	sha256Object.Write(data)
	return denied[fmt.Sprintf("%x", sha1Object.Sum(nil))] || denied[fmt.Sprintf("%x", sha256Object.Sum(nil))]
}

// A shallow boundary can hide an earlier import behind its current alias.
// Refuse it rather than treating unavailable ancestry as proof of visibility.
func assistantImportCompleteHistory(ctx context.Context, repository string) error {
	shallow, err := assistantHistoryGit(ctx, repository, nil, 16, "rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(shallow) != "false" {
		return errors.New("complete import history unavailable")
	}
	return nil
}

type assistantImportPrivacyCache struct {
	Base    string   `json:"base"`
	Objects []string `json:"objects"`
}

func assistantWorkspaceImportPrivacy(ctx context.Context, workspace *AssistantWorkspace) (map[string]bool, error) {
	data, err := assistantReadStateLimit(workspace.root, "import-privacy-v1.json", 8*1024*1024)
	if err == nil {
		var cache assistantImportPrivacyCache
		if assistantDecodeStrict(data, &cache) != nil || cache.Base != workspace.Conversation.Base || len(cache.Objects) > 100000 {
			return nil, errors.New("invalid import privacy cache")
		}
		denied := map[string]bool{}
		for _, object := range cache.Objects {
			if !assistantHistoryRevision.MatchString(object) || strings.Trim(object, "0") == "" || denied[object] {
				return nil, errors.New("invalid import privacy object")
			}
			denied[object] = true
		}
		if err := assistantImportCompleteHistory(ctx, workspace.Conversation.Repository); err != nil {
			return nil, err
		}
		return denied, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	denied, err := assistantImportedBlobs(ctx, workspace.Conversation.Repository, workspace.Conversation.Base)
	if err != nil {
		return nil, err
	}
	objects := make([]string, 0, len(denied))
	for object := range denied {
		objects = append(objects, object)
	}
	sort.Strings(objects)
	cache := assistantImportPrivacyCache{Base: workspace.Conversation.Base, Objects: objects}
	data, err = json.Marshal(cache)
	if err != nil || len(data) > 8*1024*1024 {
		return nil, errors.New("import privacy cache exceeds limit")
	}
	if err := assistantWriteState(workspace.root, "import-privacy-v1.json", data); err != nil {
		return nil, err
	}
	return denied, nil
}
