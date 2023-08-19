package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Hardcorelevelingwarrior/RSS-feed-aggregator/internal/database"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main()  {
	godotenv.Load()
	dbURL := os.Getenv("CONN")
	db, err := sql.Open("postgres",dbURL)
	if err != nil {return }
	dbQueries := database.New(db)
	cfg := apiConfig{
		DB: dbQueries,
	}
	port := os.Getenv("PORT")
	router := chi.NewRouter()
	router.Use(cors.Handler(cors.Options{}))
	//Add sub-router /v1
	a := chi.NewRouter()
	router.Mount("/v1",a)
	//Router for /v1
	a.Get("/readiness", func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, 200, map[string]interface{}{"status": "ok"})
	})
	a.Get("/err",func(w http.ResponseWriter, r *http.Request) {
		respondWithError(w,500,"Internal Server Error")
	})
	a.Post("/users",func(w http.ResponseWriter, r *http.Request) {
		request := request {}
		decoder := json.NewDecoder(r.Body)
		decoder.Decode(&request)
		param := database.CreateUserParams{
			ID: uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name: request.Name,
		}
		newUser,err := cfg.DB.CreateUser(r.Context(),param)
		if err != nil {respondWithError(w,403,err.Error());return}
		respondWithJSON(w,201,newUser)
	})
	a.Get("/users", cfg.middlewareAuth(func(w http.ResponseWriter, r *http.Request, u database.User) {
		newUsers, err := cfg.DB.GetUsersByAPIkey(r.Context(), u.ApiKey)
    if err != nil {
        respondWithError(w, 404, err.Error())
        return
    }
    respondWithJSON(w, 200, newUsers)
	}))
	a.Post("/feeds",cfg.middlewareAuth(func(w http.ResponseWriter, r *http.Request, u database.User) {
		decoder := json.NewDecoder(r.Body)
		request := request{}
		decoder.Decode(&request)
		param := database.CreateFeedParams{
			ID: uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name: request.Name,
			Url: request.URL,
			UserID: u.ID,
		}
		newFeed,err := cfg.DB.CreateFeed(r.Context(),param)
		if err != nil {respondWithError(w, 404, err.Error());return}
		paramfeedfollow := database.CreateFeedFollowParams{
			ID: uuid.New(),
			FeedID: newFeed.ID,
			UserID: u.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		feedfollow ,er:= cfg.DB.CreateFeedFollow(r.Context(),paramfeedfollow)
		if er != nil {respondWithError(w,404,er.Error());return}
		respondWithJSON(w, http.StatusCreated, map[string]interface{}{
			"feed":        newFeed,
			"feed_follow": feedfollow,
		})

	}))
	a.Get("/feeds",func(w http.ResponseWriter, r *http.Request) {
		feeds ,err:= cfg.DB.GetAllFeeds(r.Context())
		if err != nil {respondWithError(w,404,err.Error());return}
		respondWithJSON(w,200,feeds)
	})
	a.Post("/feed_follows",cfg.middlewareAuth(func(w http.ResponseWriter, r *http.Request, u database.User) {
		decoder := json.NewDecoder(r.Body)
		request := request{}
		decoder.Decode(&request)
		param := database.CreateFeedFollowParams{
			ID: uuid.New(),
			FeedID: request.Feed_id,
			UserID: u.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		feedfollow ,err := cfg.DB.CreateFeedFollow(r.Context(),param)
		if err != nil {respondWithError(w,404,err.Error())}
		respondWithJSON(w,201,feedfollow)

	}))
	a.Delete("/feed_follows/{feedFollowID}",func(w http.ResponseWriter, r *http.Request) {
		feedfollowid := chi.URLParam(r,"feedFollowID")
		id ,er := uuid.Parse(feedfollowid) 
		if er != nil {respondWithError(w,404,er.Error());return}
		err := cfg.DB.DeleteFeedFollow(r.Context(),id)
		if err != nil {respondWithError(w,404,er.Error())}

	})
	a.Get("/feed_follows",cfg.middlewareAuth(func(w http.ResponseWriter, r *http.Request, u database.User) {
		feedfollow,err := cfg.DB.GetFeedFollowForUser(r.Context(),u.ID)
		if err != nil {respondWithError(w,404,err.Error());return}
		respondWithJSON(w,200,feedfollow) 
	}))


	//Start server
	srv := http.Server{
		Addr: ":" +port,
		Handler: router,
	}
	log.Fatal(srv.ListenAndServe())
	
}

func respondWithJSON(w http.ResponseWriter, status int, payload interface{}){
	w.WriteHeader(status)
	w.Header().Add("Content-Type","application/json")
	response, err := json.Marshal(payload)
	if err != nil {
		return 
	}
	w.Write(response)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w,code,map[string]string{"error": msg})
}

type apiConfig struct {
	DB *database.Queries
}

type request struct {
	Name string `json:"name,omitempty"`
	URL string `json:"url,omitempty"`
	Feed_id uuid.UUID `json:"feed_id,omitempty"`
}
type Feed struct {
    ID            uuid.UUID  `json:"id"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
    Name          string     `json:"name"`
    URL           string     `json:"url"`
    UserID        uuid.UUID  `json:"user_id"`
    LastFetchedAt *time.Time `json:"last_fetched_at,omitempty"`
}


type authedHandler func(http.ResponseWriter, *http.Request, database.User)

func(cfg *apiConfig) middlewareAuth(handle authedHandler) http.HandlerFunc{
	return func(w http.ResponseWriter, r *http.Request) {
		api_key := strings.TrimPrefix(r.Header.Get("Authorization"),"ApiKey ")
		if api_key == ""{
			http.Error(w, "Missing API key", http.StatusUnauthorized)
            return
		}
		users,err := cfg.DB.GetUsersByAPIkey(r.Context(),api_key)
		if err != nil {respondWithError(w,404,err.Error());return}
		handle(w,r,users)
	}
}

func databaseFeedtoFeed(feed database.Feed) Feed {
	var lastFetchedAt *time.Time
    if feed.LastFetchedAt.Valid {
        lastFetchedAt = &feed.LastFetchedAt.Time
    }
    return Feed{
        ID:            feed.ID,
        CreatedAt:     feed.CreatedAt,
        UpdatedAt:     feed.UpdatedAt,
        Name:          feed.Name,
        URL:           feed.Url,
        UserID:        feed.UserID,
        LastFetchedAt: lastFetchedAt,
    }
}