package instagram_fans

import (
	"database/sql"
	"github.com/charmbracelet/log"
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
