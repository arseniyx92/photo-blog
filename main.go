package main

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	_ "github.com/go-sql-driver/mysql"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/appengine"
)

var tpl *template.Template
var db *sql.DB
var dbSession = map[string]string{}

var (
	storageClient *storage.Client

	// Set this in app.yaml when running in production.
	bucket = os.Getenv("photo-blog-282118.appspot.com")
)

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
	ctx := context.Background()
	var err error
	storageClient, err = storage.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", index)
	http.HandleFunc("/signup", signup)
	http.HandleFunc("/login", login)
	http.HandleFunc("/post", post)
	http.HandleFunc("/logout", logout)
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.Handle("/stylesheets/", http.StripPrefix("/stylesheets", http.FileServer(http.Dir("./stylesheets"))))
	http.Handle("/public/pics/", http.StripPrefix("/public/pics", http.FileServer(http.Dir("./public/images"))))
	err = http.ListenAndServe(":8080", nil)
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
	feed := Feed{}
	feed.User = dbSession[c.Value]
	c = &http.Cookie{
		Name:  "feed",
		Value: "",
	}
	if req.Method == http.MethodPost {
		mf, fh, err := req.FormFile("nf")
		check(err)
		defer mf.Close()
		// creating new file
		//connecting to gcloud
		fname, err := uploadFile(req, mf, fh)
		check(err)
		// wd, err := os.Getwd()
		// check(err)
		// path := filepath.Join(wd, "public", "images", fname)
		// nf, err := os.Create(path)
		// check(err)
		// defer nf.Close()
		// copy
		// mf.Seek(0, 0)
		// io.Copy(nf, mf)
		ctx := appengine.NewContext(req)
		obj, err := storageClient.Bucket(bucket).Object(fname).Attrs(ctx)
		check(err)
		c = appendValue(w, c, obj.MediaLink)
	}
	xs := strings.Split(c.Value, "|")
	xs = append(xs[1:])
	feed.Pics = xs
	tpl.ExecuteTemplate(w, "post.gohtml", feed)
}

func uploadFile(req *http.Request, mf multipart.File, fh *multipart.FileHeader) (string, error) {
	//getting extension
	ext, err := fileFilter(req, fh)
	check(err)
	//creating a sha
	h := sha1.New()
	io.Copy(h, mf)
	fname := fmt.Sprintf("%x", h.Sum(nil)) + "." + ext
	//putting file
	mf.Seek(0, 0)
	ctx := appengine.NewContext(req)
	return fname, putFile(ctx, fname, mf)
}

func putFile(ctx context.Context, fname string, rdr io.Reader) error {
	sw := storageClient.Bucket(bucket).Object(fname).NewWriter(ctx)
	io.Copy(sw, rdr)
	return sw.Close()
}

func fileFilter(req *http.Request, fh *multipart.FileHeader) (string, error) {
	//ext := strings.Split(fh.Filename, ".")[1]
	ext := fh.Filename[strings.LastIndex(fh.Filename, ".")+1:]

	switch ext {
	case "jpg", "jpeg", "txt", "md":
		return ext, nil
	}
	return ext, fmt.Errorf("We do not allow files of type %s. We only allow jpg, jpeg, txt, md extensions", ext)
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
	ctx := appengine.NewContext(req)
	query := &storage.Query{Prefix: ""}
	it := storageClient.Bucket(bucket).Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		c = appendValue(w, c, attrs.MediaLink)
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

	db, err := sql.Open("mysql", "arseniyx92:123@unix(/cloudsql/photo-blog-282118:europe-west6:photo-blog-users)/users?charset=utf8")
	//db, err := sql.Open("mysql", "arseniyx92:123@tcp(34.65.166.197)/users?charset=utf8")
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

	db, err := sql.Open("mysql", "arseniyx92:123@unix(/cloudsql/photo-blog-282118:europe-west6:photo-blog-users)/users?charset=utf8")
	//db, err := sql.Open("mysql", "arseniyx92:123@tcp(34.65.166.197)/users?charset=utf8")
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
