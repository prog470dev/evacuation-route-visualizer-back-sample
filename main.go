package main

import (
	"cloud.google.com/go/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
)

type Data struct {
	Id        string  `json:"id"`
	Latitude  float32 `json:"latitude"`
	Longitude float32 `json:"longitude"`
	Type      int32   `json:"type"`
}

var (
	db            *sql.DB
	storageClient *storage.Client
	bucket        = os.Getenv("GCLOUD_STORAGE_BUCKET")
)

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func getUsers(w http.ResponseWriter, r *http.Request) {
	var rows *sql.Rows

	rows, err := db.Query("select * from goapp.users;")
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not query db1: %v", err), 500)
		return
	}
	defer rows.Close()

	datas := []Data{}
	for rows.Next() {
		var data = Data{}
		if err := rows.Scan(&data.Id, &data.Latitude, &data.Longitude, &data.Type); err != nil {
			http.Error(w, fmt.Sprintf("Could not scan result2: %v", err), 500)
			return
		}
		datas = append(datas, data)
	}
	outputJson, err := json.Marshal(&datas)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(outputJson))

	return
}

func setUsers(w http.ResponseWriter, r *http.Request) {
	type Rb struct {
		ID        string `json:"id"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
		Type      string `json:"type"`
	}

	var rb Rb
	if err := json.NewDecoder(r.Body).Decode(&rb); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rows, err := db.Query("SELECT * FROM goapp.users WHERE id=?", rb.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not query db1: %v", err), 500)
		return
	}
	defer rows.Close()

	if rows.Next() { //すでに存在した場合は更新
		_, err := db.Query("UPDATE goapp.users SET latitude=?, longitude=?, type=? WHERE id=?", rb.Latitude, rb.Longitude, rb.Type, rb.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not query db1: %v", err), 500)
			return
		}
	} else {
		_, err := db.Query("INSERT INTO goapp.users (id, latitude, longitude, type) VALUES (?, ?, ?, ?)", rb.ID, rb.Latitude, rb.Longitude, rb.Type)
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not query db1: %v", err), 500)
			return
		}
	}

	return
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	f, fh, err := r.FormFile("file")
	if err != nil {
		msg := fmt.Sprintf("Could not get file: %v", err)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	defer f.Close()

	sw := storageClient.Bucket(bucket).Object(fh.Filename).NewWriter(ctx)
	sw.ContentType = "text/csv" //csvとして認識
	if _, err := io.Copy(sw, f); err != nil {
		msg := fmt.Sprintf("Could not write file: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	if err := sw.Close(); err != nil {
		msg := fmt.Sprintf("Could not put file: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	u, _ := url.Parse("/" + bucket + "/" + sw.Attrs().Name)

	fmt.Fprintf(w, "Successful! URL: https://storage.googleapis.com%s", u.EscapedPath())

	return
}

func main() {
	// for DB
	datastoreName := os.Getenv("MYSQL_CONNECTION")
	var err error
	db, err = sql.Open("mysql", datastoreName)
	if err != nil {
		log.Fatal(err)
	}

	// for Storage
	ctx := context.Background()
	storageClient, err = storage.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()

	http.HandleFunc("/_ah/health", healthCheckHandler)

	/** User **/
	r.HandleFunc("/user", getUsers).Methods("GET")
	r.HandleFunc("/user", setUsers).Methods("POST")

	/** Log **/
	r.HandleFunc("/log", uploadHandler).Methods("POST")

	log.Print("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
