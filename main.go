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
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type User struct {
	url       string
	storyLink string
	fansCount string
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

	users, err := findUserEmptyData(config.Dsn, config.Table)
	if err != nil {
		log.Fatalf("Can not find user empty data, %v", err)
	}
	log.Printf("has %d to handle", len(users))
	if len(users) == 0 {
		return
	}

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Can not run playwright, %v", err)
	}
	defer pw.Stop()

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
			updateUserInfo(users[begin:end], account, config, pw)
		}()
	}
	wg.Wait()
}

// <editor-fold desc="playwright function">
func updateUserInfo(users []*User, account Account, config *Config, pw *playwright.Playwright) {
	context, err := logInToInstagram(account.Username, account.Password, pw, config)
	if err != nil {
		log.Fatalf("Can not login to instagram, %v", err)
	}
	defer (*context.browser).Close()

	for _, user := range users {
		user.fansCount = getFansCount(context.page, user.url)
		user.storyLink = getStoriesLink(context.page, user.url)
		log.Printf("fans_count: %s, story_link: %s for %s", user.fansCount, user.storyLink, user.url)
		time.Sleep(time.Duration(config.DelayConfig.DelayForNext) * time.Millisecond)
	}
	updateDataToDb(users, config.Dsn, config.Table)
}

func logInToInstagram(userName string, password string, pw *playwright.Playwright, config *Config) (*PlayWrightContext, error) {
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false),
	})
	if err != nil {
		log.Fatalf("Can not launch browser, %v", err)
	}
	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("Can not create page, %v", err)

	}
	if _, err := page.Goto("https://www.instagram.com/accounts/login/"); err != nil {
		log.Fatalf("Can not go to login page, %v", err)
	}

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

	return &PlayWrightContext{pw: pw, browser: &browser, page: &page}, nil
}

func getFansCount(pageRef *playwright.Page, websiteUrl string) string {
	var page = *pageRef
	if _, err := page.Goto(websiteUrl); err != nil {
		log.Printf("Can not go to user page, %v", err)
		return ""
	}

	path, err := url.Parse(websiteUrl)
	if err != nil {
		log.Printf("can not parse url(%s) \n", path)
		return ""
	}

	var websitePath = path.Path
	if strings.LastIndex(websitePath, "/") != len(websitePath)-1 {
		websitePath = websitePath + "/"
	}

	targetPath := websitePath + "followers/"
	selector := fmt.Sprintf(`a[href="%s"]`, targetPath)

	aEle, err := page.QuerySelector(selector)
	if err != nil || aEle == nil {
		log.Printf("======> can not find (%s) \n", targetPath)
		return ""
	}

	content, err := aEle.TextContent()
	if err != nil {
		log.Fatalf("failed to read content")
	}

	lines := strings.Split(content, " ")
	return lines[0]
}

func getStoriesLink(pageRef *playwright.Page, url string) string {
	page := *pageRef
	storiesLink := findStoriesLink(url)
	if storiesLink == "" {
		return ""
	}
	if _, err := page.Goto(storiesLink); err != nil {
		log.Printf("Can not go to stories page, %v", err)
		return ""
	}

	newUrl := page.URL()
	if !strings.Contains(newUrl, storiesLink) {
		log.Printf("Can not go to stories page, storiesLink: %s, newUrl: %s", storiesLink, newUrl)
		return ""
	}

	content, err := page.Content()
	if err != nil {
		log.Printf("Can not read content, %v", err)
		return ""
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "\"story_link\"") {
			return parseStoryLink(line)
		}
	}
	return ""
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

func findUserEmptyData(dsn string, table string) ([]*User, error) {
	db, err := connectToDB(dsn)
	if err != nil {
		log.Fatalf("Can not connect to db, %v", err)
	}
	defer db.Close()

	queryStr := fmt.Sprintf("SELECT  url FROM %s WHERE story_link IS NULL or fans_count is NUll or fans_count = \"\" or story_link =\"\"", table)
	rows, err := db.Query(queryStr)
	if err != nil {
		log.Fatalf("Can not select db, %v ", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var url string

		err := rows.Scan(&url)
		if err != nil {
			log.Fatalf("scan error, %v", err)
		}
		users = append(users, &User{url: url})
	}
	return users, nil
}

func updateDataToDb(users []*User, dsn string, table string) {
	db, err := connectToDB(dsn)
	if err != nil {
		log.Fatalf("Can not connect to db, %v", err)
	}
	defer db.Close()

	for _, user := range users {
		if user.fansCount != "" || user.storyLink != "" {
			execStr := fmt.Sprintf("UPDATE %s SET story_link = ?, fans_count = ? WHERE url = ?", table)
			_, err := db.Exec(execStr, user.storyLink, user.fansCount, user.url)
			if err != nil {
				log.Printf("Can not update db, %v ", err)
			}
			log.Printf("update user(%s) count %s, link: %s success", user.url, user.fansCount, user.storyLink)
		}
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

func parseStoryLink(link string) string {
	key := "\"story_link\""
	idx := strings.Index(link, key)
	if idx == -1 {
		return ""
	}
	idx = idx + len(key)
	for idx < len(link) && link[idx] != '{' {
		idx++
	}
	idx++

	start := idx - 1
	if start >= len(link) {
		return ""
	}

	for idx < len(link) && link[idx] != '}' {
		idx++
	}
	end := idx + 1

	if end >= len(link) {
		return ""
	}
	jsonStr := link[start:end]
	var result map[string]interface{}

	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return ""
	}
	return result["url"].(string)
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
	for scanner.Scan() {
		line := scanner.Text()
		link := parseStoryLink(line)
		if link != "" {
			log.Printf("The link is: %s\n", link)
		}
	}
	log.Printf("err: %v", scanner.Err())
}

//</editor-fold>
