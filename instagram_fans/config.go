package instagram_fans

import (
	"encoding/json"
	"github.com/charmbracelet/log"
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
	Accounts       []Account   `json:"accounts"`
	DelayConfig    DelayConfig `json:"delay_config"`
	Dsn            string      `json:"dsn"`
	Table          string      `json:"table"`
	Count          int         `json:"count"`
	UserDsn        string      `json:"userDsn"`
	UserTable      string      `json:"userTable"`
	ParseFansCount bool        `json:"parseFansCount"`
	ParseStoryLink bool        `json:"parseStoryLink"`
	ShowBrowser    bool        `json:"showBrowser"`
}

func ParseConfig(filePath string) *Config {
	file, err := os.Open(filePath)
	if err != nil {
		log.Errorf("Can not open file, %v", err)
		return nil
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := Config{}
	err = decoder.Decode(&config)
	if err != nil {
		log.Errorf("Can not decode file, %v", err)
		return nil
	}

	if len(config.Accounts) == 0 {
		log.Errorf("No account found in config")
		return nil
	}

	if !config.ParseFansCount && !config.ParseStoryLink {
		log.Errorf("No parseFansCount and parseStoryLink found in config")
		return nil
	}
	return &config
}
