package adapters

import (
	"context"
	"encoding/json"

	"github.com/giovantenne/nixorium/internal/domain"
)

func AssistantCoreIdentity(ctx context.Context, repository, base string) (*domain.AssistantCore, error) {
	data, err := resetGit(ctx, repository, nil, 1024*1024, "show", base+":flake.lock")
	if err != nil {
		return nil, err
	}
	var lock struct {
		Root  string `json:"root"`
		Nodes map[string]struct {
			Inputs map[string]json.RawMessage `json:"inputs"`
			Locked struct {
				Type  string `json:"type"`
				Owner string `json:"owner"`
				Repo  string `json:"repo"`
				Rev   string `json:"rev"`
			} `json:"locked"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(data), &lock); err != nil {
		return nil, err
	}
	var name string
	if json.Unmarshal(lock.Nodes[lock.Root].Inputs["nixorium"], &name) != nil {
		return nil, nil
	}
	locked := lock.Nodes[name].Locked
	if locked.Type != "github" {
		return nil, nil
	}
	core := &domain.AssistantCore{Owner: locked.Owner, Repo: locked.Repo, Rev: locked.Rev}
	if !domain.ValidAssistantCore(*core) {
		return nil, nil
	}
	return core, nil
}

func (store AssistantWorkspaceStore) CoreIdentity(conversation domain.AssistantConversation) (*domain.AssistantCore, error) {
	return AssistantCoreIdentity(context.Background(), conversation.Repository, conversation.Base)
}
