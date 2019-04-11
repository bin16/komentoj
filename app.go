package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-contrib/static"
	"github.com/rs/xid"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
)

var (
	cfg appConfig
)

// for userID
func asInt(a interface{}) int {
	if a != nil {
		return a.(int)
	}

	return 0
}

func main() {
	/**** load config.toml ==> cfg ****/
	if _, err := toml.DecodeFile("config.toml", &cfg); err != nil {
		log.Fatal(err)
	}

	/**** Init database ****/
	err := initSqliteDB(cfg.App.Database)
	if err != nil {
		log.Fatal(err)
	}

	/**** cookie store ****/
	store := cookie.NewStore([]byte("Hello@"))

	/**** gin Routers ****/
	r := gin.Default()
	r.Use(sessions.Sessions("is", store))
	r.Use(static.Serve("/", static.LocalFile("./static", false)))
	r.LoadHTMLGlob("tpl/*")
	r.GET("/", func(c *gin.Context) {
		hostname := c.Query("hostname")
		target := c.Query("target")
		backURL := c.Request.URL.String()

		user, err := checkAuthed(c)
		if err != nil || user.ID == 0 {
			c.HTML(http.StatusOK, "index.html", gin.H{
				"hostname": hostname,
				"target":   target,
				"authed":   false,
				"backURL":  backURL,
			})
			return
		}

		c.HTML(http.StatusOK, "index.html", gin.H{
			"hostname": hostname,
			"target":   target,
			"authed":   true,
			"backURL":  backURL,
			"user":     user,
		})
	})
	r.GET("/comments", func(c *gin.Context) {
		hostname := c.Query("hostname")
		target := c.Query("target")

		store, err := mkDB(cfg)
		if err != nil {
			errorWithMessage(c, "* Can not open database, so [500]")
			return
		}

		// note: results  is [] when err != nil
		results, _ := store.findComments(hostname, target)
		c.JSON(http.StatusOK, results)
	})
	r.POST("/comments", func(c *gin.Context) {
		user, err := checkAuthed(c)
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}

		var ii commentRecived
		err = c.BindJSON(&ii)
		if err != nil || len(ii.Content) == 0 {
			c.Status(http.StatusBadRequest)
			return
		}
		ii.UserID = user.ID

		store, err := mkDB(cfg)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		ci, err := store.insertComment(ii)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}

		c.JSON(http.StatusCreated, ci)
	})
	r.GET("/auth/:name", func(c *gin.Context) {
		backURL := c.Query("b")
		if len(backURL) == 0 {
			errorAndStop(c, "** No backURL")
			return
		}
		state := xid.New().String()

		store, err := mkDB(cfg)
		if err != nil {
			errorWithMessage(c, "* Can not open database, so [500]")
			return
		}

		err = store.insertOAuthLog(oauthLogInput{
			State:   state,
			BackURL: backURL,
		})
		if err != nil {
			errorWithMessage(c, "* Insert failed, [500] again")
			return
		}

		name := c.Param("name")
		h := mkOAuth2Handler(name, state, cfg.OAuth2[name])
		url := h.AuthCodeURL() // state included auto ⬆️
		c.Redirect(http.StatusTemporaryRedirect, url)
	})
	r.GET("/auth/:name/callback", func(c *gin.Context) {
		name := c.Param("name")
		info := cfg.OAuth2[name]
		code := c.Query("code")
		state := c.Query("state")

		store, err := mkDB(cfg)
		if err != nil {
			errorWithMessage(c, "* Can not open database, so [500]")
			return
		}
		l, err := store.findOAuthLog(state)
		if err != nil {
			errorAndStop(c, "*** Not US, [400]")
			return
		}
		if time.Now().Unix()-l.Time.Unix() > 60*5 {
			errorWithMessage(c, "* Timeout")
			return
		}

		h := mkOAuth2Handler(name, state, info)
		err = h.GetToken(code, state)
		if err != nil {
			errorWithMessage(c, "* Can not get token")
			return
		}

		profile, err := h.GetProfile()
		if err != nil {
			errorWithMessage(c, "* Can not get profile")
			return
		}

		userImage, err := downloadUserImage(profile.Image)
		if err == nil { // NOT err
			// download user image local if can
			profile.Image = userImage
		}

		err = store.fillProfile(&profile)
		if err != nil {
			errorWithMessage(c, "* Can not open database, so [500]")
			return
		}

		sess := sessions.Default(c)
		sess.Set("UserID", profile.ID)
		sess.Save()

		c.Redirect(http.StatusTemporaryRedirect, l.BackURL)
	})
	r.GET("/logout", func(c *gin.Context) {
		backURL := c.Query("b")
		c.SetCookie("is", "", -1, "/", cfg.App.Hostname, false, true)
		c.Redirect(http.StatusTemporaryRedirect, backURL)
	})
	r.Run()
}

/****** Utils ******/

// something danger, bye
func errorAndStop(c *gin.Context, m string) {
	err := fmt.Errorf(fmt.Sprintf("*** %s", m))
	c.AbortWithError(http.StatusBadRequest, err)
}

// back to home, with ?error=xxx
func errorWithMessage(c *gin.Context, m string) {
	fmt.Println("* Something error in service, back to home")
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/?error=%s", m))
}
