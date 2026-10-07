package domain

// AssistantConfig describes the controller's installed, explicit opt-in.
type AssistantConfig struct {
	Enabled      bool
	Endpoint     string
	SubscribeURL string
}
