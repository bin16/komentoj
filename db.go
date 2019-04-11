package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func mkDB(c appConfig) (*myDB, error) {
	result := myDB{}
	db, err := sql.Open(c.Driver, fullPath(c.App.Database))
	if err != nil {
		return &result, err
	}

	return &myDB{
		db: db,
	}, nil
}

type myDB struct {
	db *sql.DB
}

type authKey struct {
	ID int // userID
}

type userInput struct {
	GithubID string `json:"github_id"`
	GoogleID string `json:"google_id"`
	Name     string `json:"name"`
	Image    string `json:"image"`
	Email    string `json:"email"`
	Blog     string `blog:"blog"`
}

type userProfile struct {
	userInput
	ID int `json:"id"`
}

func (d *myDB) findUser(col, val string) (userProfile, error) {
	user := userProfile{}
	var userID int
	var name, image string
	row := d.db.QueryRow(fmt.Sprintf(`SELECT id, name, image FROM users WHERE %s = ?`, col), val)
	err := row.Scan(&userID, &name, &image)
	if err != nil {
		return user, err // user not exists
	}

	user.ID = userID
	user.Name = name
	user.Image = image

	return user, nil
}

func (d *myDB) fillProfile(p *userProfile) error {
	var col, val string
	if val = p.GithubID; len(val) > 0 {
		col = "github_id"
	} else if val = p.GoogleID; len(val) > 0 {
		col = "google_id"
	} else {
		col = "id"
		val = strconv.Itoa(p.ID)
	}

	u, err := d.findUser(col, val)
	fmt.Println(u, "xxxxx")
	if err == nil {
		// user exists, update profile self
		p.ID = u.ID
		return nil
	}

	// user not exists, insert one
	raw := fmt.Sprintf(`INSERT INTO users (name, email, blog, image, %s) VALUES (?, ?, ?, ?, ?)`, col)
	result, err := d.db.Exec(raw, p.Name, p.Email, p.Blog, p.Image, val)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = int(id)

	return nil
}

// vistiors dont care about hostname or target
type commentType struct {
	ID      int       `json:"id"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
	Name    string    `json:"name"`  // user name
	Image   string    `json:"image"` // user image
}

// but the values are important for us
type commentRecived struct {
	UserID   int    `json:"user_id"`
	Hostname string `json:"hostname"`
	Target   string `json:"target"`
	Content  string `json:"content"`
}

func (d *myDB) findComments(hostname, target string) ([]commentType, error) {
	items := []commentType{}
	rows, err := d.db.Query(`
		SELECT
			comments.id, comments.content, comments.time,
			users.name, users.image 
		FROM
			comments INNER JOIN users
		ON
			comments.user_id = users.id
		WHERE
			hostname = ? AND target = ?`, hostname, target)
	if err != nil || rows == nil {
		return items, err
	}
	defer rows.Close()

	var id int
	var content, name, image string
	var t time.Time

	for rows.Next() {
		if err := rows.Scan(&id, &content, &t, &name, &image); err != nil {
			fmt.Println("// ignore one?", err)
			continue
		}
		item := commentType{
			ID:      id,
			Content: content,
			Time:    t,
			Name:    name,
			Image:   image,
		}

		items = append(items, item)
	}

	return items, nil
}

func (d *myDB) insertComment(c commentRecived) (commentType, error) {
	co := commentType{}
	result, err := d.db.Exec(`
		INSERT INTO comments 
			(content, target, hostname, user_id)
		VALUES
			(?, ?, ?, ?)`, c.Content, c.Target, c.Hostname, c.UserID)
	if err != nil {
		return co, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return co, err
	}

	co.ID = int(id)
	co.Content = c.Content
	co.Time = time.Now()

	return co, nil
}

func (d *myDB) insertOAuthLog(log oauthLogInput) error {
	_, err := d.db.Exec(`INSERT INTO oauth_logs (state, back_url) VALUES (?, ?)`, log.State, log.BackURL)
	if err != nil {
		return err
	}

	return nil
}

func (d *myDB) findOAuthLog(s string) (oauthLogType, error) {
	result := oauthLogType{}
	var state, backURL string
	var t time.Time
	row := d.db.QueryRow(`SELECT state, back_url, time FROM oauth_logs WHERE state = ?`, s)
	err := row.Scan(&state, &backURL, &t)
	if err != nil {
		return result, err
	}

	result.BackURL = backURL
	result.State = state
	result.Time = t

	return result, nil
}

/****** Database ******/
func initSqliteDB(database string) error {
	_, err := os.Stat(database)
	if !os.IsNotExist(err) {
		return nil
	}
	log.Println(err)

	db, err := sql.Open("sqlite3", database)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			target VARCHAR(256) NOT NULL,
			hostname VARCHAR(128),
			user_id INTEGER NOT NULL,
			time DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE oauth_logs (
		state VARCHAR(128) PRIMARY KEY,
		back_url VARCHAR(256),
		time DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64),
			email VARCHAR(128),
			blog VARCHAR(128),
			image VARCHAR(256),
			github_id VARCHAR(128),
			google_id VARCHAR(128),
			time DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	return nil
}
