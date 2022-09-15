package downloader

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/bbalet/stopwords"
	"github.com/farrej10/xeldaChallenge/configs"
	"github.com/farrej10/xeldaChallenge/internal/dbinit"
	redis "github.com/go-redis/redis/v9"
	"go.uber.org/zap"
	"jaytaylor.com/html2text"
)

type IDownloader interface {
	RandomDownload(http.ResponseWriter, *http.Request)
}
type (
	Config struct {
		Logger *zap.SugaredLogger
	}
	downloader struct {
		logger *zap.SugaredLogger
		db     *sql.DB
		rdb    *redis.Client
		ctx    context.Context
	}
)

const (
	host     = "db"
	port     = 5432
	user     = "postgres"
	password = "postgres"
	dbname   = "xelda"
)

func NewDownloader(config Config) (IDownloader, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	var err error
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		config.Logger.DPanic(err)
	}
	err = db.Ping()
	if err != nil {
		config.Logger.DPanic(err)
	}
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis" + ":" + "6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		config.Logger.DPanic(err)
	}
	config.Logger.Info(pong)
	// create Tables this is terrible but fast
	dbinit.DbInit(db, config.Logger)
	return downloader{logger: config.Logger, db: db, rdb: rdb, ctx: ctx}, nil
}

func (dl downloader) handleHttpError(rw http.ResponseWriter, req *http.Request, errIncoming error) {
	rw.WriteHeader(http.StatusInternalServerError)
	rw.Header().Set(configs.ContentType, configs.AppJson)
	resp := make(map[string]string)
	resp["error"] = errIncoming.Error()
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		dl.logger.Errorf("error in JSON marshal. Err: %s", err)
	}
	rw.Write(jsonResp)
}

func (dl downloader) RandomDownload(rw http.ResponseWriter, req *http.Request) {
	// looping 200 times just for demo this would be populated over time
	for i := 0; i < 200; i++ {
		rp, err := http.Get(configs.WikiUrl)
		if err != nil {
			dl.handleHttpError(rw, req, err)
			return
		}

		// Load the HTML document
		doc, err := goquery.NewDocumentFromReader(rp.Body)
		if err != nil {
			dl.handleHttpError(rw, req, err)
			return
		}

		// only extracting paragraphs this is a design decision for speed of dev
		var srtString string
		doc.Find("p").Each(func(i int, s *goquery.Selection) {
			// For each item found, get the title
			tmp, err := s.Html()
			if err != nil {
				dl.handleHttpError(rw, req, err)
				return
			}
			srtString = srtString + tmp
		})

		title := doc.Find("title").Text()
		dl.logger.Info(title)
		if err != nil {
			dl.handleHttpError(rw, req, err)
			return
		}
		// convert html to text for future parsing
		content, err := html2text.FromString(srtString, html2text.Options{OmitLinks: true, TextOnly: true})
		if err != nil {
			dl.handleHttpError(rw, req, err)
			return
		}

		id := 0
		err = dl.db.QueryRow(configs.InsertArticle, strings.ToLower(content), title, "https://"+rp.Request.URL.Host+rp.Request.URL.Path).Scan(&id)
		if err != nil {
			// dont do this either shouldnt return db error
			dl.handleHttpError(rw, req, err)
			return
		}

		dl.logger.Infoln(id)

		// add tokens to reverse index
		for _, token := range analyze(content) {
			ids, err := dl.rdb.Get(dl.ctx, token).Result()
			if err != nil && err != redis.Nil {
				dl.handleHttpError(rw, req, err)
				return
			}
			if err != redis.Nil && strings.Contains(ids, strconv.Itoa(id)) {
				// checkng for id already there
				continue
			}
			// this should be a redis list but im just making it a long string for speed of dev
			// no ttl
			err = dl.rdb.Set(dl.ctx, token, ids+" "+strconv.Itoa(id), 0).Err()
			if err != nil {
				dl.handleHttpError(rw, req, err)
				return
			}
		}
	}
	rw.WriteHeader(http.StatusOK)
	rw.Header().Set(configs.ContentType, configs.AppJson)
	resp := make(map[string]string)
	resp["status"] = "OK"
	//resp["link"] = "https://" + rp.Request.URL.Host + rp.Request.URL.Path
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		dl.logger.Errorf("error in JSON marshal. Err: %s", err)
	}
	rw.Write(jsonResp)
}

func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		// Split on any character that is not a letter or a number.
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

// makes all tokens lowercase
func lowercaseFilter(tokens []string) []string {
	r := make([]string, len(tokens))
	for i, token := range tokens {
		r[i] = strings.ToLower(token)
	}
	return r
}

func analyze(text string) []string {
	// remove stopwords
	cleanContent := stopwords.CleanString(text, "en", true)
	tokens := tokenize(cleanContent)
	tokens = lowercaseFilter(tokens)
	return tokens
}
