package instagram_fans

import "github.com/playwright-community/playwright-go"

type PlayWrightContext struct {
	Pw      *playwright.Playwright
	Browser *playwright.Browser
	Page    *playwright.Page
}
