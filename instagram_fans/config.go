package instagram_fans

import (
	"encoding/json"
	"os"
)

type Account struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type DelayConfig struct {
	DelayForNext    int `json:"delay_for_next"`
	DelayAfterLogin int `json:"delay_after_login"`
}

type Config struct {
	Accounts    []Account   `json:"accounts"`
	DelayConfig DelayConfig `json:"delay_config"`
	Dsn         string      `json:"dsn"`
	Table       string      `json:"table"`
	ShowBrowser bool        `json:"showBrowser"`
}

func ParseConfig(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := Config{}
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
