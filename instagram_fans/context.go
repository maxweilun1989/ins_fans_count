package instagram_fans

import (
	"github.com/playwright-community/playwright-go"
	"gorm.io/gorm"
)

type AppContext struct {
	Pw     *playwright.Playwright
	Db     *gorm.DB
	Config *Config
}
