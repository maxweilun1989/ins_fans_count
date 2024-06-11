package instagram_fans

import (
	"encoding/json"
	"github.com/charmbracelet/log"
	"os"
)

type DelayConfig struct {
	DelayForNext    int `json:"delay_for_next"`
	DelayAfterLogin int `json:"delay_after_login"`
}

type Config struct {
	AccountCount     int         `json:"accountCount"`
	DelayConfig      DelayConfig `json:"delay_config"`
	Dsn              string      `json:"dsn"`
	Table            string      `json:"table"`
	SimilarUserTable string      `json:"similarUserTable"`
	Count            int         `json:"count"`
	MaxCount         int         `json:"maxCount"`
	AccountDSN       string      `json:"accountDsn"`
	AccountTable     string      `json:"accountTable"`
	ParseFansCount   bool        `json:"parseFansCount"`
	ParseStoryLink   bool        `json:"parseStoryLink"`
	ShowBrowser      bool        `json:"showBrowser"`
	ErrorSavePercent int         `json:"errSavePercent"`
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

	if config.AccountCount == 0 {
		log.Errorf("No account found in config")
		return nil
	}

	if !config.ParseFansCount && !config.ParseStoryLink {
		log.Errorf("No parseFansCount and parseStoryLink found in config")
		return nil
	}
	return &config
}
