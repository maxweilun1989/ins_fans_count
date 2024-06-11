package instagram_fans

import (
	"github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/playwright-community/playwright-go"
	"net/url"
	"os"
	"path/filepath"
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
	ErrPageTimeout     = errors.New("page timeout")
	ErrPageNoEleFound  = errors.New("no element found")

	PageTimeOut = time.Duration(60)
	PageWait    = 30

	httpErrorText         = "HTTP ERROR"
	suspendAccountText    = "We suspended your account"
	helpConfirmText       = "Help us confirm it"
	pageNotValidText      = "Sorry, this page isn't available."
	homeSelector          = `svg[aria-label="Home"]`
	dismissSelector       = `role=button >> text=Dismiss`
	usernameInputSelector = "input[name='username']"
	followersSelector     = `a:has-text("followers"), button:has-text("followers")`
	bodySelector          = "body"
	logInSelector         = `a:has-text("Log In"), button:has-text("Log In")`

	similarBloggerButton = `svg[aria-label="Similar accounts"]`
	seeAllButton         = `a:has-text("See all"), button:has-text("See all")`
	suggestedFriends     = `div > div >h1:has-text("Suggested for you")`
	linkRole             = "role=link"
)

var httpErrorCondition TextCondition
var suspendedAccountCondition TextCondition
var suspicionsLoginCondition TextCondition
var passwordIncorrectCondition TextCondition
var helpConfirmCondition TextCondition
var pageNotValidCondition TextCondition

var homeCondition ElementCondition
var dismissSelectorCondition ElementCondition
var usernameInputCondition ElementCondition
var followersCondition ElementCondition
var bodyElementCondition ElementCondition

var similarBloggerButtonSelector ElementCondition
var seeAllButtonSelector ElementCondition
var suggestedFriendsCondition ElementCondition
var loginCondition ElementCondition

func init() {
	httpErrorCondition = TextCondition{Text: httpErrorText}
	suspendedAccountCondition = TextCondition{Text: suspendAccountText}
	suspicionsLoginCondition = TextCondition{Text: "Suspicious Login Attempt"}
	passwordIncorrectCondition = TextCondition{Text: "your password was incorrect"}
	helpConfirmCondition = TextCondition{Text: helpConfirmText}
	pageNotValidCondition = TextCondition{Text: pageNotValidText}

	homeCondition = ElementCondition{Selector: homeSelector}
	dismissSelectorCondition = ElementCondition{Selector: dismissSelector}
	usernameInputCondition = ElementCondition{Selector: usernameInputSelector}
	followersCondition = ElementCondition{Selector: followersSelector}
	bodyElementCondition = ElementCondition{Selector: bodySelector}

	similarBloggerButtonSelector = ElementCondition{Selector: similarBloggerButton}
	seeAllButtonSelector = ElementCondition{Selector: seeAllButton}
	suggestedFriendsCondition = ElementCondition{Selector: suggestedFriends}
	loginCondition = ElementCondition{Selector: logInSelector}
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
		log.Errorf("[Login] Can not go to Login Page, %v", err)
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

		cond, err := CommonHandleCondition(page, homeCondition, i, maxLoginCount, account, "login")
		log.Infof("[Login] account[%s] Condition %v, err: %v", account.Username, cond, err)

		if err != nil {
			return err
		}

		if cond == homeCondition {
			log.Infof("[Login] success login!!!")
			return nil
		}
	}
	return nil
}

func GetFansCount(pageRef *playwright.Page, websiteUrl string, account *Account) (int, error) {
	var page = *pageRef
	maxCount := 2

	for i := 0; i < maxCount; i++ {
		if _, err := page.Goto(websiteUrl, playwright.PageGotoOptions{
			Timeout: playwright.Float(float64(time.Second * PageTimeOut / time.Millisecond)),
		}); err != nil {
			log.Errorf("[GetFansCount] Can not go to user page, %v", err)
			return -1, ErrUserInvalid
		}

		fillCond, err := CommonHandleCondition(pageRef, followersCondition, i, maxCount, account, "fans_count")

		if err != nil {
			return -2, err
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
	}

	return -2, errors.Errorf("No followers found")
}

func GetStoriesLink(pageRef *playwright.Page, webSiteUrl string, account *Account) (string, error) {
	page := *pageRef

	storiesLink := findStoriesLink(webSiteUrl)
	if storiesLink == "" {
		return "", nil
	}

	maxCount := 2
	for i := 0; i < maxCount; i++ {
		if _, err := page.Goto(storiesLink, playwright.PageGotoOptions{
			Timeout: playwright.Float(float64(time.Second * PageTimeOut / time.Millisecond)),
		}); err != nil {
			log.Printf("[GetStoriesLink] Can not go to stories page, %v", err)
			return "", nil
		}

		fillCond, err := CommonHandleCondition(pageRef, bodyElementCondition, i, maxCount, account, "story_link")
		log.Infof("[GetStoriesLink] Condition %v, err %v", fillCond, err)
		if err != nil {
			return "", ErrUserInvalid
		}

		if fillCond == bodyElementCondition {
			content, err := page.Content()
			if err != nil {
				log.Printf("[GetStoriesLink] Can not read content, %v", err)
				return "", err
			}

			lines := strings.Split(content, "\n")
			result := parseStoryLikes(lines)
			if len(result) == 0 {
				return "", err
			}
			return strings.Join(result, ","), nil
		}
	}
	return "", errors.Errorf("No stories found")
}

func FetchSimilarBloggers(pageRef *playwright.Page, webSiteUrl string, account *Account, user *UserSimilarFriends, saveError bool) error {
	tag := "similar_blogger"
	maxCount := 2
	for i := 0; i < maxCount; i++ {
		_, err := (*pageRef).Goto(webSiteUrl)
		if err != nil {
			log.Errorf("[FetchSimlarBlogger] Can not go to user page, %v", err)
			return err
		}

		fillCond, err := clickButton(pageRef, similarBloggerButtonSelector, i, maxCount, account, tag)
		if err != nil {
			if saveError {
				_ = savePage(pageRef, webSiteUrl+".html")
			}
			if errors.Is(err, playwright.ErrTimeout) {
				user.Status = 404
			}
			return err
		}

		if fillCond == nil {
			continue
		}
		time.Sleep(time.Duration(1) * time.Second)

		fillCond, err = clickButton(pageRef, seeAllButtonSelector, i, maxCount, account, tag)

		if err != nil {
			if saveError {
				_ = savePage(pageRef, webSiteUrl+".html")
			}
			if errors.Is(err, playwright.ErrTimeout) {
				user.Status = 404
			}
			return err
		}

		if fillCond == nil {
			continue
		}
		time.Sleep(time.Duration(1) * time.Second)

		fillCond, err = CommonHandleCondition(pageRef, suggestedFriendsCondition, i, maxCount, account, tag)
		if err != nil {
			if saveError {
				_ = savePage(pageRef, webSiteUrl+".html")
			}
			return err
		}

		if fillCond == nil {
			continue
		}

		element, err := (*pageRef).QuerySelector(suggestedFriendsCondition.Selector)
		if err != nil {
			return err
		}

		dialogElement, err := element.QuerySelector("xpath=../..")
		if err != nil {
			return err
		}

		links, err := dialogElement.QuerySelectorAll(linkRole)
		if err != nil {
			return err
		}
		set := make(map[string]struct{})
		for _, link := range links {
			href, err := link.GetAttribute("href")
			if err != nil {
				continue
			}
			href = strings.TrimRight(strings.TrimLeft(href, "/"), "/")
			set[href] = struct{}{}
		}

		arr := make([]string, len(set))
		idx := 0
		for k := range set {
			arr[idx] = k
			idx += 1
		}
		user.SimilarFriends = strings.Join(arr, ",")
		log.Infof("[FetchSimlarBlogger] Found similar bloggers: %s ", user.SimilarFriends)
		user.Status = 200
		return nil
	}
	user.Status = 404
	return nil
}

func clickButton(pageRef *playwright.Page, testCond ElementCondition, curIdx int, maxCount int, account *Account, tag string) (Condition, error) {
	fillCond, err := CommonHandleCondition(pageRef, testCond, curIdx, maxCount, account, tag)
	if err != nil {
		return nil, err
	}

	if fillCond == nil {
		return nil, ErrPageNoEleFound
	}

	if fillCond == testCond {
		selector, err := (*pageRef).QuerySelector(testCond.Selector)
		if err != nil || selector == nil {
			log.Errorf("[FetchSimlarBlogger] Can not find similar blogger button, %v, selecotr: %v", err, selector)
			return nil, ErrPageNoEleFound
		}

		err = selector.Click()
		if err != nil {
			log.Errorf("[FetchSimlarBlogger] Can notClick similar blogger button, %v", err)
			return nil, err
		}
	}
	return fillCond, nil
}

func CommonHandleCondition(page *playwright.Page, testCond Condition, curIdx int, maxCount int, account *Account, tag string) (Condition, error) {

	idleCondition := StatusCondition{State: playwright.LoadStateDomcontentloaded}
	cond, err := WaitForConditions(page,
		[]Condition{
			passwordIncorrectCondition,
			pageNotValidCondition,
			httpErrorCondition,
			suspendedAccountCondition,
			suspicionsLoginCondition,
			dismissSelectorCondition,
			usernameInputCondition,
			helpConfirmCondition,
			loginCondition,
			idleCondition,
			testCond,
		}, float64(PageWait))

	log.Infof("[CommonHandleCondition.%s] Condition: %v, err: %v, account:%s", tag, cond, err, account.Username)

	if err != nil {
		return nil, ErrPageTimeout
	}
	if cond == testCond {
		return testCond, nil
	}

	if cond == idleCondition {
		// document ready, test again
		log.Infof("[CommonHandleCondition.%s] Document ready, test again", tag)
		_, err := testCond.Wait(page, 5)
		if err != nil {
			log.Errorf("[CommonHandleCondition.%s] Retest failed: %v", tag, err)
			return nil, ErrPageNoEleFound
		}
		return testCond, nil
	}

	if cond == passwordIncorrectCondition ||
		cond == suspicionsLoginCondition ||
		cond == suspendedAccountCondition ||
		cond == httpErrorCondition {
		return nil, ErrUserInvalid
	}

	if cond == pageNotValidCondition {
		return nil, ErrPageUnavailable
	}

	if cond == helpConfirmCondition {
		return nil, ErrUserUnusable
	}

	if cond == dismissSelectorCondition {
		dismissButton, err := (*page).QuerySelector(dismissSelector)
		if err != nil {
			return nil, ErrUserInvalid
		}
		if dismissButton != nil {
			if err := dismissButton.Click(); err != nil {
				log.Error("[CommonHandleCondition] Can not click dismiss button")
				return nil, ErrUserInvalid
			}
			log.Info("[CommonHandleCondition] Click dismiss button, sleep 5s and test again")
			time.Sleep(time.Duration(5) * time.Second)
			cond, err := WaitForConditions(page, []Condition{testCond}, float64(PageWait))
			if err != nil {
				log.Errorf("[CommonHandleCondition] Can not wait for test condition, %v", err)
				return nil, ErrPageTimeout
			}
			if cond == testCond {
				return testCond, nil
			}
		}
	}

	if cond == loginCondition {
		if curIdx == maxCount-1 {
			return nil, ErrNeedLogin
		}
		time.Sleep(time.Duration(3) * time.Second)
		err = Login(account, page)
		if err != nil {
			return nil, err
		}
	}

	if cond == usernameInputCondition {
		if curIdx == maxCount-1 {
			return nil, ErrUserInvalid
		}
		return nil, nil
	}
	return nil, nil
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

func savePage(page *playwright.Page, webSiteUrl string) error {
	htmlContent, err := (*page).Content()
	if err != nil {
		return err
	}
	folder := "errors"

	if _, err := os.Stat(folder); err != nil {
		err := os.Mkdir(folder, 0775)
		if err != nil {
			return err
		}
	}

	parse, err := url.Parse(webSiteUrl)
	if err != nil {
		return err
	}

	filePath := filepath.Join(folder, parse.Path+".html")
	err = os.WriteFile(filePath, []byte(htmlContent), 0666)
	if err != nil {
		return err
	}
	return nil
}
