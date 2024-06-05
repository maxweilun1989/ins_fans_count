package instagram_fans

import (
	"github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/playwright-community/playwright-go"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUserInvalid     = errors.New("user is invalid")
	ErrNeedLogin       = errors.New("need login")
	ErrUserUnusable    = errors.New("user is unusable")
	ErrPageUnavailable = errors.New("page is unavailable")

	PageTimeOut = time.Duration(20)

	helpConfirmText       = "Help us confirm it"
	pageNotValidText      = "Sorry, this page isn't available."
	homeSelector          = `svg[aria-label="Home"]`
	dismissSelector       = `role=button >> text=Dismiss`
	usernameInputSelector = "input[name='username']"
	followersSelector     = `a:has-text("followers"), button:has-text("followers")`
)

var suspicionsLoginCondition TextCondition
var passwordIncorrectCondition TextCondition
var helpConfirmCondition TextCondition
var pageNotValidCondition TextCondition

var homeCondition ElementCondition
var dismissSelectorCondition ElementCondition
var usernameInputCondition ElementCondition
var followersCondition ElementCondition

func init() {

	suspicionsLoginCondition = TextCondition{Text: "Suspicious Login Attempt"}
	passwordIncorrectCondition = TextCondition{Text: "your password was incorrect"}
	helpConfirmCondition = TextCondition{Text: helpConfirmText}
	pageNotValidCondition = TextCondition{Text: pageNotValidText}

	homeCondition = ElementCondition{Selector: homeSelector}
	dismissSelectorCondition = ElementCondition{Selector: dismissSelector}
	usernameInputCondition = ElementCondition{Selector: usernameInputSelector}
	followersCondition = ElementCondition{Selector: followersSelector}
}

func NewBrowser(pw *playwright.Playwright) (*playwright.Browser, error) {
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false),
	})
	if err != nil {
		log.Fatalf("Can not launch Browser, %v", err)
	}
	return &browser, err
}

func NewPage(browser *playwright.Browser) (*playwright.Page, error) {
	contextOptions := playwright.BrowserNewContextOptions{
		Locale: playwright.String("en-US"), // 设置语言为简体中文
	}
	context, err := (*browser).NewContext(contextOptions)
	if err != nil {
		log.Fatalf("failed to set local")
		return nil, err
	}

	page, err := context.NewPage()
	if err != nil {
		log.Fatalf("Can not create Page, %v", err)

	}
	return &page, err
}

func LogInToInstagram(account *Account, page *playwright.Page) error {
	if _, err := (*page).Goto("https://www.instagram.com/accounts/login/", playwright.PageGotoOptions{
		Timeout: playwright.Float(float64(time.Second * PageTimeOut / time.Millisecond)),
	}); err != nil {
		log.Fatalf("Can not go to Login Page, %v", err)
		return err
	}

	return Login(account, page)
}

func Login(account *Account, page *playwright.Page) error {
	maxLoginCount := 2
	for i := 0; i < maxLoginCount; i++ {
		inputName := "input[name='username']"
		if err := (*page).Fill(inputName, account.Username); err != nil {
			log.Errorf("[Login] Can not fill username, %v", err)
			return ErrUserInvalid
		}
		time.Sleep(1 * time.Second)

		inputPass := "input[name='password']"
		if err := (*page).Fill(inputPass, account.Password); err != nil {
			log.Errorf("[Login] Can not fill password, %v", err)
			return ErrUserInvalid
		}
		time.Sleep(1 * time.Second)

		submitBtn := "button[type='submit']"
		if err := (*page).Click(submitBtn); err != nil {
			log.Errorf("[Login] Can not click submit btn, %v", err)
			return ErrUserInvalid
		}

		time.Sleep(10 * time.Second)

		cond, err := WaitForConditions(page,
			[]Condition{passwordIncorrectCondition,
				suspicionsLoginCondition,
				dismissSelectorCondition,
				usernameInputCondition,
				helpConfirmCondition,
				homeCondition,
			})
		if err != nil {
			return ErrUserInvalid
		}

		log.Infof("[Login] account[%s] Condition %v", account.Username, cond)

		if cond == homeCondition {
			return nil
		}

		if cond == passwordIncorrectCondition || cond == suspicionsLoginCondition {
			return ErrUserInvalid
		}

		if cond == helpConfirmCondition {
			return ErrUserUnusable
		}

		if cond == dismissSelectorCondition {
			dismissButton, err := (*page).QuerySelector(dismissSelector)
			if err != nil {
				return ErrUserInvalid
			}
			if dismissButton != nil {
				if err := dismissButton.Click(); err != nil {
					return ErrUserInvalid
				}
				time.Sleep(time.Duration(5) * time.Second)
				return nil
			}
		}

		if cond == usernameInputCondition {
			if i == maxLoginCount-1 {
				return ErrUserInvalid
			}
			continue
		}
		break
	}
	return nil
}

func GetFansCount(pageRef *playwright.Page, websiteUrl string) (int, error) {
	var page = *pageRef
	maxCount := 2

	for i := 0; i < maxCount; i++ {
		if _, err := page.Goto(websiteUrl, playwright.PageGotoOptions{
			Timeout: playwright.Float(float64(time.Second * PageTimeOut / time.Millisecond)),
		}); err != nil {
			log.Errorf("[GetFansCount] Can not go to user page, %v", err)
			return -1, ErrUserInvalid
		}

		conditions := []Condition{
			suspicionsLoginCondition,
			helpConfirmCondition,
			followersCondition,
			dismissSelectorCondition,
			pageNotValidCondition,
		}

		fillCond, err := WaitForConditions(pageRef, conditions)
		if err != nil {
			log.Errorf("[GetFansCount] can not wait for selector finished %v", err)
			if errors.Is(err, playwright.ErrTimeout) {
				return -2, ErrUserInvalid
			}
		}

		log.Infof("[GetFansCount] Condition %v", fillCond)

		if fillCond == pageNotValidCondition {
			return -2, ErrPageUnavailable
		}

		if fillCond == passwordIncorrectCondition || fillCond == suspicionsLoginCondition {
			return -2, ErrUserInvalid
		}

		if fillCond == helpConfirmCondition {
			return -2, ErrUserUnusable
		}

		if fillCond == followersCondition {
			elements, err := page.QuerySelectorAll(followersSelector)
			if err != nil {
				log.Errorf("[getFansCount] could not query selector: %v", err)
				return -2, ErrUserUnusable
			}

			for _, element := range elements {
				textContent, err := element.TextContent()
				if err != nil {
					log.Errorf("could not get text content: %v", err)
				}

				parts := strings.Split(textContent, " ")

				countStr := parts[0]

				count, err := ParseFollowerCount(countStr)
				if err != nil {
					log.Errorf("---- falied to convert %s to int(%v)", countStr, err)
					continue
				}
				return count, nil
			}
		}

		if fillCond == dismissSelectorCondition {
			dismissButton, err := page.QuerySelector(dismissSelector)
			if err != nil {
				log.Errorf("[GetFansCount] could not query selector: %v", err)
				return -1, ErrUserUnusable
			}
			if err := dismissButton.Click(); err != nil {
				log.Errorf("[GetFansCount] could not click dismiss button: %v", err)
				return -1, ErrUserUnusable
			}
			time.Sleep(time.Duration(5) * time.Second)
			continue
		}
	}

	return -2, errors.Errorf("No followers found")
}

func GetStoriesLink(pageRef *playwright.Page, webSiteUrl string) (string, error) {
	page := *pageRef
	storiesLink := findStoriesLink(webSiteUrl)
	if storiesLink == "" {
		return "", nil
	}

	if _, err := page.Goto(storiesLink, playwright.PageGotoOptions{
		Timeout: playwright.Float(float64(time.Second * PageTimeOut / time.Millisecond)),
	}); err != nil {
		log.Printf("Can not go to stories page, %v", err)
		return "", nil
	}

	if _, err := page.WaitForSelector("body"); err != nil {
		log.Printf("wait for body show failed: %v", err)
	}

	newUrl := page.URL()

	if strings.Contains(newUrl, "https://www.instagram.com/accounts/login/") {
		return "", ErrNeedLogin
	}

	content, err := page.Content()
	if err != nil {
		log.Printf("Can not read content, %v", err)
		return "", err
	}

	lines := strings.Split(content, "\n")
	result := parseStoryLikes(lines)
	if len(result) == 0 {
		return "", err
	}
	return strings.Join(result, ","), nil
}

func parseStoryLikes(lines []string) []string {
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
	return result
}

func findStoriesLink(site string) string {
	parsedUrl, err := url.Parse(site)
	if err != nil {
		return ""
	}
	newPath := "stories" + parsedUrl.Path
	return parsedUrl.Scheme + "://" + parsedUrl.Host + "/" + newPath
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
