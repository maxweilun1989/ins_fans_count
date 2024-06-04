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
	ErrUserInvalid  = errors.New("user is invalid")
	ErrNeedLogin    = errors.New("need login")
	ErrUserUnusable = errors.New("user is unusable")

	PageFinishEleFound  = 1
	PageFinishStateIdle = 2

	PageTimeOut = time.Duration(20)
)

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
	for i := 0; i < 2; i++ {
		inputName := "input[name='username']"
		if err := (*page).Fill(inputName, account.Username); err != nil {
			log.Errorf("Can not fill username, %v", err)
			return ErrUserInvalid
		}
		time.Sleep(1 * time.Second)

		inputPass := "input[name='password']"
		if err := (*page).Fill(inputPass, account.Password); err != nil {
			log.Errorf("Can not fill password, %v", err)
			return ErrUserInvalid
		}
		time.Sleep(1 * time.Second)

		submitBtn := "button[type='submit']"
		if err := (*page).Click(submitBtn); err != nil {
			log.Errorf("Can not click submit btn, %v", err)
			return ErrUserInvalid
		}

		err := (*page).WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State: playwright.LoadStateNetworkidle,
		})
		if err != nil {
			log.Errorf("Can not wait for load state, %v", err)
			return ErrUserInvalid
		}

		time.Sleep(time.Duration(1000) * time.Millisecond)

		pageContent, err := (*page).Content()
		if err != nil {
			log.Errorf("Can not get page content, %v", err)
			return ErrUserInvalid
		}

		targetText := "Suspicious Login Attempt"
		if strings.Contains(pageContent, targetText) {
			log.Errorf("[%v] Suspicious Login Attempt found! ", *account)
			return ErrUserInvalid
		} else if strings.Contains(pageContent, "your password was incorrect") {
			log.Errorf("[%v] your password was incorrect", *account)
			return ErrUserInvalid
		}

		time.Sleep(time.Duration(5) * time.Second)

		dismissSelector := `role=button >> text=Dismiss`
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

		inputEle, err := (*page).QuerySelector(inputName)
		if err != nil {
			return ErrUserInvalid
		}
		if inputEle != nil {
			continue
		}
		break
	}
	return nil
}

func GetFansCount(pageRef *playwright.Page, websiteUrl string) (int, error) {
	var page = *pageRef
	if _, err := page.Goto(websiteUrl, playwright.PageGotoOptions{
		Timeout: playwright.Float(float64(time.Second * PageTimeOut / time.Millisecond)),
	}); err != nil {
		log.Printf("Can not go to user page, %v", err)
	}

	selector := `a:has-text("followers"), button:has-text("followers")`
	_, err := page.WaitForSelector(selector)
	if err != nil {
		log.Errorf("can not wait for selector finished %v", err)
		if errors.Is(err, playwright.ErrTimeout) {
			return -1, commonErrorHandle(pageRef)
		}
	}

	elements, err := page.QuerySelectorAll(selector)
	if err != nil {
		log.Errorf("could not query selector: %v", err)
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

	return -1, errors.Errorf("")
}

func commonErrorHandle(page *playwright.Page) error {

	siteUrl := (*page).URL()
	if strings.Contains(siteUrl, "https://www.instagram.com/accounts/login/") {
		return ErrNeedLogin
	}

	content, err := (*page).Content()
	if err != nil {
		return ErrUserInvalid
	}

	if strings.Contains(content, "Page Not Found") || strings.Contains(content, "Page Not Found • Instagram") {
		return ErrUserUnusable
	}

	if strings.Contains(content, "Suspicious Login Attempt") {
		return ErrUserInvalid
	} else if strings.Contains(content, "your password was incorrect") {
		return ErrUserInvalid
	}

	time.Sleep(time.Duration(5) * time.Second)

	dismissSelector := `role=button >> text=Dismiss`
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
	return ErrUserInvalid
}

func GetStoriesLink(pageRef *playwright.Page, webSiteUrl string, account *Account) (string, error) {
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
