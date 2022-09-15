package main

import (
	"database/sql"
	"math/rand"
	"net/http"
	"time"

	"github.com/farrej10/xeldaChallenge/internal/downloader"
	"github.com/farrej10/xeldaChallenge/internal/search"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var sugar *zap.SugaredLogger
var dl downloader.IDownloader
var s search.ISearch
var db *sql.DB

func init() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar = logger.Sugar()
	rand.Seed(time.Now().UnixNano())

	var err error
	dl, err = downloader.NewDownloader(downloader.Config{Logger: sugar})
	if err != nil {
		sugar.DPanic(err)
	}
	s, err = search.NewSearch(search.Config{Logger: sugar})
	if err != nil {
		sugar.DPanic(err)
	}
}

func main() {
	sugar.Info("Starting Http Server")
	http.HandleFunc("/", dl.RandomDownload)
	http.HandleFunc("/search", s.SearchHandler)
	http.ListenAndServe(":8080", nil)
	defer db.Close()
}
