package main

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	uuid "github.com/satori/go.uuid"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var tpl *template.Template
var db *sql.DB
var dbSession = map[string]string{}

//Feed is ...
type Feed struct {
	User string
	Pics []string
}

//User is ...
type User struct {
	NameErr  int
	Err      int
	Name     string
	Email    string
	Password string
}

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/signup", signup)
	http.HandleFunc("/login", login)
	http.HandleFunc("/post", post)
	http.HandleFunc("/logout", logout)
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.Handle("/stylesheets/", http.StripPrefix("/stylesheets", http.FileServer(http.Dir("./stylesheets"))))
	http.Handle("/public/pics/", http.StripPrefix("/public/pics", http.FileServer(http.Dir("./public/images"))))
	err := http.ListenAndServe(":8080", nil)
	check(err)
}

func post(w http.ResponseWriter, req *http.Request) {
	//getting and checking cookie
	c, err := req.Cookie("session")
	if err == http.ErrNoCookie {
		http.Redirect(w, req, "/", http.StatusSeeOther)
	} else if _, ok := dbSession[c.Value]; !ok {
		http.Redirect(w, req, "/", http.StatusSeeOther)
	}
	// main
	if req.Method == http.MethodPost {
		mf, fh, err := req.FormFile("nf")
		check(err)
		defer mf.Close()
		// creating a sha
		ext := strings.Split(fh.Filename, ".")[1]
		h := sha1.New()
		io.Copy(h, mf)
		fname := fmt.Sprintf("%x", h.Sum(nil)) + "." + ext
		// creating new file
		wd, err := os.Getwd()
		check(err)
		path := filepath.Join(wd, "public", "images", fname)
		nf, err := os.Create(path)
		check(err)
		defer nf.Close()
		// copy
		mf.Seek(0, 0)
		io.Copy(nf, mf)
		c = appendValue(w, c, fname)
	}
	xs := strings.Split(c.Value, "|")
	tpl.ExecuteTemplate(w, "post.gohtml", xs)
}

func appendValue(w http.ResponseWriter, c *http.Cookie, fname string) *http.Cookie {
	s := c.Value
	if !strings.Contains(s, fname) {
		s += "|" + fname
	}
	c.Value = s
	http.SetCookie(w, c)
	return c
}

func logout(w http.ResponseWriter, req *http.Request) {
	c, err := req.Cookie("session")
	if err == http.ErrNoCookie {
		http.Redirect(w, req, "/", http.StatusSeeOther)
	}
	c.MaxAge = -1
	http.SetCookie(w, c)
	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func index(w http.ResponseWriter, req *http.Request) {
	// c, err := req.Cookie("session")
	// if err == http.ErrNoCookie {
	// 	tpl.ExecuteTemplate(w, "index.gohtml", nil)
	// 	return
	// }
	// tpl.ExecuteTemplate(w, "index.gohtml", dbSession[c.Value])
	feed := Feed{}
	c, err := req.Cookie("session")
	if err == http.ErrNoCookie {
	} else if _, ok := dbSession[c.Value]; !ok {
	} else {
		feed.User = dbSession[c.Value]
	}
	c = &http.Cookie{
		Name:  "feed",
		Value: "",
	}
	wd, err := os.Getwd()
	check(err)
	path := filepath.Join(wd, "public", "images")
	files, err := ioutil.ReadDir(path)
	check(err)
	for _, file := range files {
		c = appendValue(w, c, file.Name())
	}
	xs := strings.Split(c.Value, "|")
	xs = append(xs[1:])
	feed.Pics = xs
	tpl.ExecuteTemplate(w, "index.gohtml", feed)
}

func signup(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		tpl.ExecuteTemplate(w, "signup.gohtml", nil)
		return
	}
	un := req.FormValue("name")
	email := req.FormValue("email")
	pas := req.FormValue("password")
	pas1 := req.FormValue("password1")
	pas2, err := bcrypt.GenerateFromPassword([]byte(pas), bcrypt.MinCost)
	check(err)

	CurUser := User{
		NameErr:  2,
		Err:      0,
		Name:     un,
		Email:    email,
		Password: string(pas2),
	}

	db, err := sql.Open("mysql", "arseniyx92:123@tcp(34.65.166.197)/users?charset=utf8")
	check(err)
	defer db.Close()
	err = db.Ping()
	check(err)
	// insert, err := db.Query(`INSERT INTO users VALUES('hp', 'myhp@gmail.com')`)
	// check(err)
	// defer insert.Close()

	rows, err := db.Query(`SELECT UserName FROM users WHERE UserName='` + CurUser.Name + `'`)
	defer rows.Close()
	check(err)
	var name string
	cnt := 0
	for rows.Next() {
		err = rows.Scan(&name)
		check(err)
		cnt++
	}

	if cnt > 0 {
		CurUser.Err = 2
		tpl.ExecuteTemplate(w, "signup.gohtml", CurUser)
		return
	}

	if pas != pas1 {
		//TODO
		CurUser.Err = 1
		// fmt.Println(CurUser.Email)
		tpl.ExecuteTemplate(w, "signup.gohtml", CurUser)
		return
	}

	insert, err := db.Query(`INSERT INTO users VALUES('` + CurUser.Name + `', '` + CurUser.Email + `', '` + CurUser.Password + `')`)
	check(err)
	defer insert.Close()

	GenCookie(w, CurUser.Name)
	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func login(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		tpl.ExecuteTemplate(w, "login.gohtml", nil)
		return
	}
	un := req.FormValue("name")
	password := req.FormValue("password")
	pas2, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	check(err)

	CurUser := User{
		NameErr:  2,
		Err:      0,
		Name:     un,
		Email:    "",
		Password: string(pas2),
	}

	db, err := sql.Open("mysql", "arseniyx92:123@tcp(34.65.166.197)/users?charset=utf8")
	check(err)
	defer db.Close()
	err = db.Ping()
	check(err)
	rows, err := db.Query(`SELECT Password FROM users WHERE UserName='` + CurUser.Name + `'`)
	defer rows.Close()
	check(err)
	var pas string
	cnt := 0
	for rows.Next() {
		err := rows.Scan(&pas)
		check(err)
		cnt++
	}

	if cnt == 0 {
		CurUser.Err = 2
		tpl.ExecuteTemplate(w, "login.gohtml", CurUser)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(pas), []byte(password))
	if err != nil {
		CurUser.Err = 1
		tpl.ExecuteTemplate(w, "login.gohtml", CurUser)
		return
	}

	GenCookie(w, CurUser.Name)
	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// GenCookie ...
func GenCookie(w http.ResponseWriter, name string) {
	SId, _ := uuid.NewV4()
	c := &http.Cookie{
		Name:  "session",
		Value: SId.String(),
	}
	dbSession[SId.String()] = name
	c.MaxAge = 120
	http.SetCookie(w, c)
}
