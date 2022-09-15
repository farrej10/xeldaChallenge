package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/bbalet/stopwords"
	"github.com/farrej10/xeldaChallenge/configs"
	"github.com/farrej10/xeldaChallenge/internal/dbinit"
	"github.com/go-redis/redis/v9"
	"go.uber.org/zap"
)

type ISearch interface {
	SearchHandler(http.ResponseWriter, *http.Request)
}
type (
	Config struct {
		Logger *zap.SugaredLogger
	}
	search struct {
		logger *zap.SugaredLogger
		db     *sql.DB
		rdb    *redis.Client
		ctx    context.Context
	}

	Article struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Link    string `json:"link"`
		content string
	}
	Rank struct {
		Article Article `json:"article"`
		Rank    int     `json:"rank"`
	}
)

const (
	host     = "db"
	port     = 5432
	user     = "postgres"
	password = "postgres"
	dbname   = "xelda"
)

func NewSearch(config Config) (ISearch, error) {
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
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		config.Logger.DPanic(err)
	}
	// create Tables this is terrible but fast
	dbinit.DbInit(db, config.Logger)
	return search{logger: config.Logger, db: db, rdb: rdb, ctx: ctx}, nil
}

func (s search) handleHttpError(rw http.ResponseWriter, req *http.Request, errIncoming error) {
	rw.WriteHeader(http.StatusInternalServerError)
	rw.Header().Set(configs.ContentType, configs.AppJson)
	resp := make(map[string]string)
	resp["error"] = errIncoming.Error()
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		s.logger.Errorf("error in JSON marshal. Err: %s", err)
	}
	rw.Write(jsonResp)
}

func (s search) SearchHandler(rw http.ResponseWriter, req *http.Request) {

	// rows, err := s.db.Query(configs.SelectArticles)
	// if err != nil {
	// 	s.handleHttpError(rw, req, err)
	// 	return
	// }
	// defer rows.Close()

	query := req.URL.Query().Get("query")

	// var articles []Article

	// for rows.Next() {
	// 	var art Article
	// 	if err := rows.Scan(&art.content, &art.Title, &art.Link); err != nil {
	// 		s.handleHttpError(rw, req, err)
	// 		return
	// 	}
	// 	articles = append(articles, art)
	// }
	ranks, err := s.rank(query)
	if err != nil {
		s.handleHttpError(rw, req, err)
		return
	}

	body, err := json.Marshal(ranks)
	if err != nil {
		s.handleHttpError(rw, req, err)
		return
	}

	rw.WriteHeader(http.StatusOK)
	rw.Header().Set(configs.ContentType, configs.AppJson)
	rw.Write(body)
}

func (s search) rank(query string) ([]Rank, error) {
	if query == "" {
		return nil, errors.New("no query found")
	}
	query = strings.ToLower(query)

	idsPerToken, err := s.searchIndex(query)
	s.logger.Info(idsPerToken)
	if err != nil {
		return nil, err
	}

	feqmap := make(map[string]int)
	for _, ids := range idsPerToken {
		for _, id := range ids {
			if f, ok := feqmap[id]; ok {
				feqmap[id] = f + 1
			} else {
				feqmap[id] = 1
			}
		}
	}
	s.logger.Info(feqmap)
	var ranks []Rank
	for i, instances := range feqmap {
		var article Article
		intId, err := strconv.Atoi(i)
		if err != nil {
			return nil, err
		}
		article.ID = i
		if err := s.db.QueryRow(configs.SelectArticleById, intId).Scan(&article.Title, &article.Link); err != nil {
			return nil, err
		}
		ranks = append(ranks, Rank{Rank: instances, Article: article})
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		return ranks[i].Rank > ranks[j].Rank
	})
	return ranks, nil
}

func (s search) searchIndex(text string) ([][]string, error) {
	var r [][]string
	for _, token := range analyze(text) {
		val, err := s.rdb.Get(s.ctx, token).Result()
		if err != nil && err != redis.Nil {
			return nil, nil
		}
		if err != redis.Nil {
			r = append(r, strings.Fields(val))
		}
	}
	return r, nil
}

// all these should be in a util package
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
