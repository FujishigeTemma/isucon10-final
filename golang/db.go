package xsuportal

import (
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"log"
	"time"

	"github.com/isucon/isucon10-final/webapp/golang/util"
)

func GetDB() (*sqlx.DB, error) {
	mysqlConfig := mysql.NewConfig()
	mysqlConfig.Net = "tcp"
	mysqlConfig.Addr = util.GetEnv("MYSQL_HOSTNAME", "127.0.0.1") + ":" + util.GetEnv("MYSQL_PORT", "3306")
	mysqlConfig.User = util.GetEnv("MYSQL_USER", "isucon")
	mysqlConfig.Passwd = util.GetEnv("MYSQL_PASS", "isucon")
	mysqlConfig.DBName = util.GetEnv("MYSQL_DATABASE", "xsuportal")
	mysqlConfig.Params = map[string]string{
		"time_zone": "'+00:00'",
	}
	mysqlConfig.ParseTime = true
	mysqlConfig.InterpolateParams = true

	dbx, err := sqlx.Open("mysql", mysqlConfig.FormatDSN())
	if err != nil {
		return nil, err
	}

	dbx.SetMaxIdleConns(1024) // デフォルトだと2
	dbx.SetConnMaxLifetime(0) // 一応セット
	dbx.SetConnMaxIdleTime(0) // 一応セット go1.15以上

	return dbx, nil
}

func WaitDB(db *sqlx.DB) {
	for {
		err := db.Ping()
		if err == nil {
			return
		}

		log.Printf("Failed to ping DB: %s", err)
		log.Println("Retrying...")
		time.Sleep(time.Second)
	}
}

func PollDB(db *sqlx.DB) {
	for {
		err := db.Ping()
		if err != nil {
			log.Printf("Failed to ping DB: %s", err)
		}

		time.Sleep(time.Second)
	}
}
