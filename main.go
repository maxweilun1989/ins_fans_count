package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"github.com/playwright-community/playwright-go"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type User struct {
	url       string
	storyLink string
	fansCount int
}

type PlayWrightContext struct {
	pw      *playwright.Playwright
	browser *playwright.Browser
	page    *playwright.Page
}

func main() {
	log.Printf("start")
	//insertFilesToDb("./assets/blogs.txt")

	users, err := findUserEmptyData()
	if err != nil {
		log.Fatalf("Can not find user empty data, %v", err)
	}
	log.Printf("has %d to handle", len(users))

	context, err := logInToInstagram("651878205@qq.com", "Wl@19890919")
	if err != nil {
		log.Fatalf("Can not login to instagram, %v", err)
	}
	defer (*context.browser).Close()
	defer context.pw.Stop()

	url := "https://www.instagram.com/jonatasbacciotti/"
	getFansCount(context.page, url)

	storiesLink := getStoriesLink(url)
	if storiesLink == "" {
		return
	}
	if _, err := (*context.page).Goto(storiesLink); err != nil {
		log.Printf("Can not go to stories page, %v", err)
		return
	}

	newUrl := (*context.page).URL()
	if !strings.Contains(newUrl, storiesLink) {
		log.Printf("Can not go to stories page, %v", err)
		return
	}

	content, err := (*context.page).Content()
	if err != nil {
		log.Printf("Can not read content, %v", err)
		return
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "\"story_link\"") {
			log.Printf("Found stories: %s", line)
		}
	}
}

// <editor-fold desc="playwright function">
func logInToInstagram(userName string, password string) (*PlayWrightContext, error) {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Can not run playwright, %v", err)
	}
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

	time.Sleep(10 * time.Second)

	return &PlayWrightContext{pw: pw, browser: &browser, page: &page}, nil
}

func getFansCount(pageRef *playwright.Page, url string) int {
	var page = *pageRef
	if _, err := page.Goto(url); err != nil {
		log.Printf("Can not go to user page, %v", err)
		return -1
	}

	content, err := page.Content()
	if err != nil {
		log.Fatalf("failed to read content")
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "follower_count") {
			return parseFansCount(line)
		}
	}
	return -1
}

//</editor-fold>

// <editor-fold desc="db functions">
func connectToDB() (*sql.DB, error) {
	dsn := "root:19890919@tcp(localhost:3306)/instagram?timeout=2s"
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

func findUserEmptyData() ([]User, error) {
	db, err := connectToDB()
	if err != nil {
		log.Fatalf("Can not connect to db, %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT  url FROM user WHERE story_link IS NULL or fans_count is NUll")
	if err != nil {
		log.Fatalf("Can not select db, %v ", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var url string

		err := rows.Scan(&url)
		if err != nil {
			log.Fatalf("scan error, %v", err)
		}
		users = append(users, User{url: url})
	}
	return users, nil
}

func insertFilesToDb(path string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Can not open file(%s), %v", path, err)
	}
	defer file.Close()

	db, err := connectToDB()
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

func getStoriesLink(site string) string {
	parsedUrl, err := url.Parse(site)
	if err != nil {
		return ""
	}
	newPath := "stories" + parsedUrl.Path
	return parsedUrl.Scheme + "://" + parsedUrl.Host + "/" + newPath
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
