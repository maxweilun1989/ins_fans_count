package instagram_fans

import (
	"database/sql"
	"github.com/playwright-community/playwright-go"
)

type AppContext struct {
	Pw     *playwright.Playwright
	Db     *sql.DB
	Config *Config
}
