package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

type githubProfile struct {
	ID        int    `json:"id"`
	Login     string `json:"login"` // login name, fallback of name
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
	Blog      string `json:"blog"`
	URL       string `json:"url"` // url, fallback of blog
}

type googleProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Email   string `json:"email"`
	Blog    string `blog:"blog"`
}

func downloadUserImage(imageURL string) (string, error) {
	resultPath := ""
	rootDir := fullPath(cfg.App.StaticDir)
	imgDir := cfg.App.UserImageDir // name
	resp, err := http.Get(imageURL)
	if err != nil {
		fmt.Println(err)
		return resultPath, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return resultPath, err
	}

	mimeType := http.DetectContentType(body)
	imgName := mkImgName(mimeType, imageURL)
	imgPath := path.Join(rootDir, imgDir, imgName)
	absURL := path.Join("/", imgDir, imgName)
	imgFile, err := os.OpenFile(imgPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		fmt.Println(err)
		return resultPath, err
	}
	_, err = imgFile.Write(body)
	if err != nil {
		fmt.Println(err)
		return resultPath, err
	}

	return absURL, nil
}

func mkImgName(mimeType, imgURL string) string {
	urlPath := imgURL
	u, err := url.Parse(imgURL)
	if err == nil {
		urlPath = u.Path // remove ?xxx=xxx
	}
	base := path.Base(urlPath)               // xxx.png, xxx
	ext := path.Ext(base)                    // .png, ""
	bareName := base[0 : len(base)-len(ext)] // xxx
	extName := ext                           // .png OR ""

	rt := rand.New(rand.NewSource(time.Now().Unix()))
	w := rt.Int()
	randName := strconv.Itoa(w)
	baseName := bareName + "_" + randName // xxx_RANDOM

	splited := strings.Split(mimeType, "/")
	if len(splited) == 2 && splited[0] == "image" {
		extName = "." + splited[1] // .png
	}

	return path.Join(baseName + extName)
}

func getProfile(accessToken, providerName string) (userProfile, error) {
	if providerName == "google" {
		return getProfileFromGoogle(accessToken)
	}

	return getProfileFromGithub(accessToken)
}

func getProfileFromGithub(accessToken string) (userProfile, error) {
	uri := cfg.OAuth2["github"].ProfileURL + accessToken

	res, err := http.Get(uri)
	if err != nil {
		return userProfile{}, err
	}
	defer res.Body.Close()

	var gh githubProfile
	err = json.NewDecoder(res.Body).Decode(&gh)
	if err != nil {
		return userProfile{}, err
	}

	profile := userProfile{
		userInput: userInput{
			GithubID: strconv.Itoa(gh.ID),
			Name:     gh.Login, // use login name ... forget name
			Email:    gh.Email,
			Image:    gh.AvatarURL,
			Blog:     gh.URL, // use github url ... forget blog
		},
	}

	return profile, nil
}

func getProfileFromGoogle(accessToken string) (userProfile, error) {
	uri := cfg.OAuth2["google"].ProfileURL + accessToken

	res, err := http.Get(uri)
	if err != nil {
		return userProfile{}, err
	}
	defer res.Body.Close()

	var gh googleProfile
	err = json.NewDecoder(res.Body).Decode(&gh)
	fmt.Println(gh, "Google!!!")
	if err != nil {
		return userProfile{}, err
	}

	profile := userProfile{
		userInput: userInput{
			GoogleID: gh.ID,
			Name:     gh.Name,
			Email:    gh.Email,
			Image:    gh.Picture,
			Blog:     gh.Blog,
		},
	}

	return profile, nil
}

/****** Check Auth ******/

func checkAuthed(c *gin.Context) (userProfile, error) {
	user := userProfile{}

	sess := sessions.Default(c)
	userID := asInt(sess.Get("UserID"))
	store, err := mkDB(cfg)
	if err != nil {
		return user, nil
	}

	if userID == 0 {
		return user, errors.New("[401]")
	}

	user, err = store.findUser("id", strconv.Itoa(userID))
	if err != nil {
		return user, nil
	}

	return user, nil
}

/****** Config type, and oauth config ******/

type oauth2Info struct {
	Key         string `json:"key"`
	Secret      string `json:"secret"`
	RedirectURL string `json:"redirectURL"`
	ProfileURL  string `json:"profileURL"`
	Name        string `json:"name"`
}

type appInfo struct {
	Hostname     string `json:"hostname"`
	Port         string `json:"port"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Database     string `json:"database"`
	StaticDir    string `json:"static_dir"`
	UserImageDir string `json:"user_image_dir"`
}

type appConfig struct {
	Driver   string                `json:"driver"`
	Hostname string                `json:"hostname"`
	OAuth2   map[string]oauth2Info `json:"oauth2"`
	App      appInfo               `json:"app"`
}

type oauthLogInput struct {
	State   string `json:"state"`
	BackURL string `json:"back_url"`
}

type oauthLogType struct {
	oauthLogInput
	Time time.Time `json:"time"`
}

/****** OAuth2 Handler ******/

func mkOAuth2Handler(name, state string, info oauth2Info) *oauth2Handler {
	conf := oauth2.Config{
		Endpoint:     getEndpoint(name),
		ClientID:     info.Key,
		ClientSecret: info.Secret,
		RedirectURL:  info.RedirectURL,
		Scopes:       []string{},
	}
	if name == "google" {
		conf.Scopes = []string{
			"https://www.googleapis.com/auth/userinfo.profile",
		}
	}

	return &oauth2Handler{
		name:  name,
		conf:  conf,
		info:  info,
		state: state,
	}
}

type oauth2Handler struct {
	name        string
	conf        oauth2.Config
	info        oauth2Info
	state       string
	accessToken *oauth2.Token
}

func (o *oauth2Handler) GetProfile() (userProfile, error) {
	if o.name == "google" {
		return getProfileFromGoogle(o.accessToken.AccessToken)
	}

	return getProfileFromGithub(o.accessToken.AccessToken)
}

func (o *oauth2Handler) AuthCodeURL() string {
	return o.conf.AuthCodeURL(o.state, oauth2.AccessTypeOffline)
}

func (o *oauth2Handler) GetToken(code, state string) error {
	tok, err := o.conf.Exchange(context.Background(), code)
	if err != nil {
		return err
	}

	o.accessToken = tok
	return nil
}

func getEndpoint(name string) oauth2.Endpoint {
	endpoints := make(map[string]oauth2.Endpoint)
	endpoints["github"] = github.Endpoint
	endpoints["google"] = google.Endpoint

	return endpoints[name]
}
