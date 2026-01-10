package config

type AppConfig struct {
	MaxConnections       int
	AnonymousMod         int
	HidingTraffic        int
	DefaultAddressListen string
}

func GetCFG() *AppConfig {
	cfg := &AppConfig{}

	return cfg
}
