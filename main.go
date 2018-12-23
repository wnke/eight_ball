package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	//_ "github.com/jinzhu/gorm/dialects/sqlite"
	"fmt"
	"github.com/caarlos0/env"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"
)

//global database access for the callbacks
var db *gorm.DB

var answers = [...]string{
	"It is certain.",
	"It is decidedly so.",
	"Without a doubt.",
	"Yes - definitely.",
	"You may rely on it.",
	"As I see it, yes.",
	"Most likely.",
	"Outlook good.",
	"Yes.",
	"Signs point to yes.",
	"Reply hazy, try again.",
	"Ask again later.",
	"Better not tell you now.",
	"Cannot predict now.",
	"Concentrate and ask again.",
	"Don't count on it.",
	"My reply is no.",
	"My sources say no.",
	"Outlook not so good.",
	"Very doubtful."}

func RandomAnswer() string {
	return answers[rand.Intn(len(answers))]
}

type Interaction struct {
	gorm.Model `json:"-"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

type Question struct {
	Question string `json:"question"`
}

func GetReady(w http.ResponseWriter, r *http.Request) {

	var count uint

	if err := db.Model(&Interaction{}).Limit(1).Count(&count).Error; err != nil {
		log.Printf("[GetReady] Faild to retrieve any data")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func GetHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func PostQuestion(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)

	var request Question
	err = json.Unmarshal(body, &request)
	if err != nil {
		log.Printf("[PostQuestion] Failed to parse request '%s': %s", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	interaction := Interaction{Question: request.Question, Answer: RandomAnswer()}

	db.Create(&interaction)
	db.Save(&interaction)
	if db.NewRecord(interaction) {
		log.Printf("[PostQuestion] Failed to save request '%s'", string(body))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	payload, err := json.Marshal(interaction)
	if err != nil {
		log.Printf("[PostQuestion] Failed to create response '%s': %s", string(payload), err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(payload)

}

func GetQuestions(w http.ResponseWriter, r *http.Request) {
	var questionList []Interaction

	if err := db.Find(&questionList).Error; err != nil {
		log.Printf("[GetQuestions] Faild to retrieve any data")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	payload, err := json.Marshal(&questionList)
	if err != nil {
		log.Printf("[GetQuestions] Failed to create response '%s': %s", string(payload), err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(payload)

}

type config struct {
	Host string `env:"EIGHTBALL_HOST" envDefault:"0.0.0.0"`
	Port string `env:"EIGHTBALL_PORT" envDefault:"8080"`

	DatabaseHost   string `env:"EIGHTBALL_DATABASE_HOST" envDefault:"pgsql"`
	DatabasePort   string `env:"EIGHTBALL_DATABASE_PORT" envDefault:"5432"`
	DatabaseUser   string `env:"EIGHTBALL_DATABASE_USER" envDefault:"postgres"`
	DatabasePass   string `env:"EIGHTBALL_DATABASE_PASS" envDefault:"root"`
	DatabaseDbName string `env:"EIGHTBALL_DATABASE_DBNAME" envDefault:"postgres"`
}

func main() {
	var err error

	cfg := config{}
	err = env.Parse(&cfg)
	if err != nil {
		log.Fatal("[main] Failed to load env vars")
	}

	conection := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=disable",
		cfg.DatabaseHost,
		cfg.DatabasePort,
		cfg.DatabaseUser,
		cfg.DatabaseDbName,
		cfg.DatabasePass)

	db, err = gorm.Open("postgres", conection)
	if err != nil {
		log.Fatalf("[main] Failed to open database: '%s'!", err)
	}
	defer db.Close()
	db.AutoMigrate(&Interaction{})

	rand.Seed(time.Now().UnixNano())

	router := mux.NewRouter()
	router.HandleFunc("/questions", PostQuestion).Methods("POST")
	router.HandleFunc("/questions", GetQuestions).Methods("GET")
	router.HandleFunc("/health", GetHealth).Methods("GET")
	router.HandleFunc("/readiness", GetReady).Methods("GET")

	srv := &http.Server{
		Addr: cfg.Host + ":" + cfg.Port,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      router, // Pass our instance of gorilla/mux in.
	}
	// Run our server in a goroutine so that it doesn't block.
	log.Printf("[main] Serving on  '%s:%s'", cfg.Host, cfg.Port)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("[main] Shutting down!")
	os.Exit(0)
}
