package configs

const (
	InsertArticle     = `INSERT INTO articles (content,title,link) VALUES ($1,$2,$3) RETURNING id`
	SelectArticles    = `SELECT content,title,link FROM articles LIMIT 200`
	SelectArticleById = `SELECT title,link FROM articles WHERE id = $1`
)
