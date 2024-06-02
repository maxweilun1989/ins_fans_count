package instagram_fans

import (
	"github.com/charmbracelet/log"
	"github.com/playwright-community/playwright-go"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

func LogInToInstagram(userName string, password string, pw *playwright.Playwright, config *Config) (*PlayWrightContext, error) {
	headless := !config.ShowBrowser
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		log.Fatalf("Can not launch Browser, %v", err)
	}
	contextOptions := playwright.BrowserNewContextOptions{
		Locale: playwright.String("en-US"), // 设置语言为简体中文
	}
	context, err := browser.NewContext(contextOptions)
	if err != nil {
		log.Fatalf("failed to set local")
	}

	page, err := context.NewPage()
	if err != nil {
		log.Fatalf("Can not create Page, %v", err)

	}
	if _, err := page.Goto("https://www.instagram.com/accounts/login/"); err != nil {
		log.Fatalf("Can not go to Login Page, %v", err)
	}

	Login(userName, password, page, config)

	return &PlayWrightContext{Pw: pw, Browser: &browser, Page: &page}, nil
}

func Login(userName string, password string, page playwright.Page, config *Config) {
	inputName := "input[name='username']"
	if err := page.Fill(inputName, userName); err != nil {
		log.Fatalf("Can not fill username, %v", err)
	}
	time.Sleep(1 * time.Second)

	inputPass := "input[name='password']"
	if err := page.Fill(inputPass, password); err != nil {
		log.Fatalf("Can not fill password, %v", err)
	}
	time.Sleep(1 * time.Second)

	submitBtn := "button[type='submit']"
	if err := page.Click(submitBtn); err != nil {
		log.Fatalf("Can not fill password, %v", err)
	}

	time.Sleep(time.Duration(config.DelayConfig.DelayAfterLogin) * time.Millisecond)
}

func GetFansCount(pageRef *playwright.Page, websiteUrl string) int {
	var page = *pageRef
	if _, err := page.Goto(websiteUrl); err != nil {
		log.Printf("Can not go to user page, %v", err)
	}

	_, err := page.WaitForSelector("body")
	if err != nil {
		log.Printf("can not wait for selector finished %v", err)
	}

	elements, err := page.QuerySelectorAll(`a:has-text("followers"), button:has-text("followers")`)
	if err != nil {
		log.Fatalf("could not query selector: %v", err)
	}

	for _, element := range elements {
		textContent, err := element.TextContent()
		if err != nil {
			log.Printf("could not get text content: %v", err)
		}

		parts := strings.Split(textContent, " ")

		countStr := parts[0]

		count, err := ParseFollowerCount(countStr)
		if err != nil {
			log.Printf("---- falied to convert %s to int(%v)", countStr, err)
			continue
		}
		return count
	}
	return -1
}

func GetStoriesLink(pageRef *playwright.Page, webSiteUrl string, account *Account, config *Config) string {
	page := *pageRef
	storiesLink := findStoriesLink(webSiteUrl)
	if storiesLink == "" {
		return ""
	}

	var hasError = true
	for i := 0; i < 2; i++ {
		if _, err := page.Goto(storiesLink); err != nil {
			log.Printf("Can not go to stories page, %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if _, err := page.WaitForSelector("body"); err != nil {
			log.Printf("wait for body show failed: %v", err)
		}

		newUrl := page.URL()

		if strings.Contains(newUrl, "https://www.instagram.com/accounts/login/") {
			Login(account.Username, account.Password, page, config)
			time.Sleep(2 * time.Second)
			continue
		}

		if strings.Contains(newUrl, storiesLink) {
			hasError = false
			break
		}
	}

	if hasError {
		return ""
	}
	content, err := page.Content()
	if err != nil {
		log.Printf("Can not read content, %v", err)
		return ""
	}

	lines := strings.Split(content, "\n")
	result := make([]string, 0)

	for _, line := range lines {
		if strings.Contains(line, "\"story_link\"") {
			links := parseStoryLinks(line)
			if len(links) == 0 {
				continue
			}

			for _, link := range links {
				parsedLink := parseLink(link)
				if !slices.Contains(result, parsedLink) {
					result = append(result, parsedLink)
				}
			}
		}
	}

	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, ",")
}

func findStoriesLink(site string) string {
	parsedUrl, err := url.Parse(site)
	if err != nil {
		return ""
	}
	newPath := "stories" + parsedUrl.Path
	return parsedUrl.Scheme + "://" + parsedUrl.Host + "/" + newPath
}

func ParseFansCount(line string) int {
	key := "follower_count"
	idx := strings.Index(line, key)
	if idx == -1 {
		log.Print("found follower_count")
		return -1
	}
	idx = idx + len(key)
	for idx < len(line) && line[idx] != ':' {
		idx++
	}
	idx++
	if idx >= len(line) {
		return -1
	}
	start := idx
	for idx < len(line) && line[idx] != ',' {
		idx++
	}
	if idx >= len(line) {
		return -1
	}
	end := idx
	fansCount := line[start:end]

	if result, err := strconv.Atoi(fansCount); err == nil {
		return result
	}
	return -1
}

func parseStoryLinks(link string) []string {
	re := regexp.MustCompile(`"story_link":{"url":"(.*?)"}`)

	matches := re.FindAllStringSubmatch(link, -1)
	result := make([]string, len(matches))
	for idx, match := range matches {
		if len(match) >= 2 {
			result[idx] = match[1]
		}
	}
	return result
}

func parseLink(link string) string {
	linkUrl, err := url.Parse(link)
	if err != nil {
		return link
	}
	linkQuery, err := url.ParseQuery(linkUrl.RawQuery)
	if err != nil || !linkQuery.Has("u") {
		return link
	}
	uStr := linkQuery.Get("u")
	decodedUrl, err := strconv.Unquote(`"` + uStr + `"`)
	if err != nil {
		return uStr
	}
	unescapeQuery, err := url.QueryUnescape(decodedUrl)
	if err != nil {
		return decodedUrl
	}
	final, err := url.PathUnescape(unescapeQuery)

	if err != nil {
		return unescapeQuery
	}
	return final
}
