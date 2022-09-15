package dbinit

import (
	"database/sql"

	"go.uber.org/zap"
)

func DbInit(db *sql.DB, logger *zap.SugaredLogger) {
	_, err := db.Exec("DROP TABLE articles;")
	if err != nil {
		logger.DPanic(err)
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS articles (id SERIAL PRIMARY KEY, link TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL)")
	if err != nil {
		logger.DPanic(err)
	}
}
