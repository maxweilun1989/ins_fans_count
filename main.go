package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/playwright-community/playwright-go"
	"log"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// update user set fans_count = -1 where story_link = "" or story_link is null;
// view-source:https://www.instagram.com/stories/cl3milson/
//

type User struct {
	id        int
	url       string
	storyLink string
	fansCount int
}

type PlayWrightContext struct {
	pw      *playwright.Playwright
	browser *playwright.Browser
	page    *playwright.Page
}

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

func main() {
	log.Printf("start")

	config, err := parseConfig()
	if err != nil {
		log.Fatalf("Can not parse config, %v", err)
	}
	//insertFilesToDb("./assets/blogs.txt", config.Dsn)
	//return
	if len(config.Accounts) == 0 {
		log.Fatalf("No account found")

	}

	db, err := connectToDB(config.Dsn)
	if err != nil {
		log.Fatalf("failed to conenct to database %s, error: %v", config.Dsn, err)
	}
	defer db.Close()

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Can not run playwright, %v", err)
	}
	defer pw.Stop()

	low := 0
	limit := 500

	for {
		users, err := findUserEmptyData(db, config.Table, limit, low)
		if err != nil {
			log.Fatalf("Can not find user empty data, %v", err)
		}
		log.Printf("has %d to handle, low -> %d ", len(users), low)
		if len(users) == 0 {
			break
		}
		low = users[len(users)-1].id
		updateData(users, config, pw, db)
	}
}

func updateData(users []*User, config *Config, pw *playwright.Playwright, db *sql.DB) {
	if len(users) == 0 {
		return
	}

	perSize := len(users) / len(config.Accounts)
	var wg sync.WaitGroup
	for i, account := range config.Accounts {
		begin := i * perSize
		end := (i + 1) * perSize
		if end > len(users) {
			end = len(users)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateUserInfo(users[begin:end], account, config, pw, db)
		}()
	}
	wg.Wait()
}

// <editor-fold desc="playwright function">
func updateUserInfo(users []*User, account Account, config *Config, pw *playwright.Playwright, db *sql.DB) {
	context, err := logInToInstagram(account.Username, account.Password, pw, config)
	if err != nil {
		log.Fatalf("Can not login to instagram, %v", err)
	}
	defer (*context.browser).Close()

	for _, user := range users {
		user.fansCount = getFansCount(context.page, user.url)
		user.storyLink = getStoriesLink(context.page, user.url, &account, config)
		log.Printf("fans_count: %d, story_link: %s for %s", user.fansCount, user.storyLink, user.url)
		updateSingleDataToDb(user, db, config.Table)
		time.Sleep(time.Duration(config.DelayConfig.DelayForNext) * time.Millisecond)
	}
}

func logInToInstagram(userName string, password string, pw *playwright.Playwright, config *Config) (*PlayWrightContext, error) {
	headless := !config.ShowBrowser
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		log.Fatalf("Can not launch browser, %v", err)
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
		log.Fatalf("Can not create page, %v", err)

	}
	if _, err := page.Goto("https://www.instagram.com/accounts/login/"); err != nil {
		log.Fatalf("Can not go to login page, %v", err)
	}

	login(userName, password, page, config)

	return &PlayWrightContext{pw: pw, browser: &browser, page: &page}, nil
}

func login(userName string, password string, page playwright.Page, config *Config) {
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

func getFansCount(pageRef *playwright.Page, websiteUrl string) int {
	var page = *pageRef
	if _, err := page.Goto(websiteUrl); err != nil {
		log.Printf("Can not go to user page, %v", err)
	}

	_, err := page.WaitForSelector("main")
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

		count, err := parseFollowerCount(countStr)
		if err != nil {
			log.Printf("---- falied to convert %s to int(%v)", countStr, err)
			continue
		}
		return count
	}
	return -1

}

func getStoriesLink(pageRef *playwright.Page, webSiteUrl string, account *Account, config *Config) string {
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
			login(account.Username, account.Password, page, config)
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

//</editor-fold>

// <editor-fold desc="db functions">
func connectToDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Can not open db(%s), %v", dsn, err)
	}

	db.SetConnMaxLifetime(100)
	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(10)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, err
}

func findUserEmptyData(db *sql.DB, table string, limit int, low int) ([]*User, error) {
	queryStr := fmt.Sprintf("SELECT id, url FROM %s WHERE fans_count = -1 and id > %d order by id ASC limit %d", table, low, limit)
	rows, err := db.Query(queryStr)
	if err != nil {
		log.Fatalf("Can not select db, %v ", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var url string
		var id int

		err := rows.Scan(&id, &url)
		if err != nil {
			log.Fatalf("scan error, %v", err)
		}
		updateStr := fmt.Sprintf("UPDATE %s SET fans_count = -2 WHERE url = ?", table)
		_, updateErr := db.Exec(updateStr, url)
		if updateErr != nil {
			log.Printf("Can not update db for %s, %v ", url, updateErr)
		}
		users = append(users, &User{id: id, url: url})
	}
	return users, nil
}

func updateDataToDb(users []*User, db *sql.DB, table string) {
	for _, user := range users {
		updateSingleDataToDb(user, db, table)
	}
}

func updateSingleDataToDb(user *User, db *sql.DB, table string) {
	if user.fansCount != -1 || user.storyLink != "" {
		execStr := fmt.Sprintf("UPDATE %s SET story_link = ?, fans_count = ? WHERE url = ?", table)
		_, err := db.Exec(execStr, user.storyLink, user.fansCount, user.url)
		if err != nil {
			log.Printf("Can not update db, %v ", err)
		}
		log.Printf("update user(%s) count %d, link: %s success", user.url, user.fansCount, user.storyLink)
	}
}

func insertFilesToDb(path string, dsn string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Can not open file(%s), %v", path, err)
	}
	defer file.Close()

	db, err := connectToDB(dsn)
	if err != nil {
		log.Fatalf("Can not connect to db, %v", err)
	}
	defer db.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("Read line: %s", line)

		_, err := db.Exec("INSERT INTO user(url) VALUES (?)", line)
		if err != nil {
			log.Fatalf("Can not insert db, %v ", err)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Can not read file(%s), %v", path, err)
	}
}

//</editor-fold>

// <editor-fold desc="parse text helpers">
func parseFansCount(line string) int {
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

func findStoriesLink(site string) string {
	parsedUrl, err := url.Parse(site)
	if err != nil {
		return ""
	}
	newPath := "stories" + parsedUrl.Path
	return parsedUrl.Scheme + "://" + parsedUrl.Host + "/" + newPath
}

func parseConfig() (*Config, error) {
	file, err := os.Open("config.json")
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

func parseFollowerCount(s string) (int, error) {
	// 去掉字符串中的空格
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")

	// 检查字符串的最后一个字符以确定单位
	length := len(s)
	if length == 0 {
		return 0, fmt.Errorf("invalid format")
	}

	unit := s[length-1]
	numberStr := s[:length-1]
	var multiplier int

	switch unit {
	case 'K', 'k':
		multiplier = 1000
	case 'M', 'm':
		multiplier = 1000000
	case 'B', 'b':
		multiplier = 1000000000
	default:
		// 如果没有单位，尝试直接转换为整数
		numberStr = s
		multiplier = 1
	}

	// 将数字部分转换为浮点数
	number, err := strconv.ParseFloat(numberStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: %v", err)
	}

	// 计算实际的粉丝数
	followers := int(number * float64(multiplier))
	return followers, nil
}

func testParseCount() {
	file, err := os.Open("./assets/fans_count.txt")
	if err != nil {
		log.Fatalf("Can not open file, %v", err)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		count := parseFansCount(line)
		log.Printf("The count is: %d\n", count)
	}
}

func testParseStoryLink() {
	file, err := os.Open("./assets/story_link.txt")
	if err != nil {
		log.Fatalf("Can not open file, %v", err)
	}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)
	result := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()

		links := parseStoryLinks(line)

		if len(links) == 0 {
			continue
		}

		for _, link := range links {
			linkUrl, err := url.Parse(link)
			if err != nil {
				result = append(result, link)
				continue
			}
			linkQuery, err := url.ParseQuery(linkUrl.RawQuery)
			if err != nil || !linkQuery.Has("u") {
				result = append(result, link)
				continue
			}
			uStr := linkQuery.Get("u")
			decodedUrl, err := strconv.Unquote(`"` + uStr + `"`)
			if err != nil {
				result = append(result, uStr)
				continue
			}

			unescapeQuery, err := url.QueryUnescape(decodedUrl)
			if err != nil {
				result = append(result, decodedUrl)
				continue
			}
			final, err := url.PathUnescape(unescapeQuery)
			if err != nil {
				result = append(result, unescapeQuery)
				continue
			}
			result = append(result, final)

		}
	}
	log.Println(strings.Join(result, ","))
}

//</editor-fold>
