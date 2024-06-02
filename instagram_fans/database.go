package instagram_fans

import (
	"bufio"
	"database/sql"
	"fmt"
	"github.com/charmbracelet/log"
	"os"
)

func ConnectToDB(dsn string) (*sql.DB, error) {
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

func FindUserEmptyData(db *sql.DB, table string, limit int, low int) ([]*User, error) {
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
		users = append(users, &User{Id: id, Url: url})
	}
	return users, nil
}

func UpdateSingleDataToDb(user *User, db *sql.DB, table string) {
	execStr := fmt.Sprintf("UPDATE %s SET story_link = ?, fans_count = ? WHERE url = ?", table)
	_, err := db.Exec(execStr, user.StoryLink, user.FansCount, user.Url)
	if err != nil {
		log.Printf("Can not update db, %v ", err)
	}
	log.Printf("update user(%s) count %d, link: %s success", user.Url, user.FansCount, user.StoryLink)
}

func InsertFilesToDb(path string, dsn string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Can not open file(%s), %v", path, err)
	}
	defer file.Close()

	db, err := ConnectToDB(dsn)
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
