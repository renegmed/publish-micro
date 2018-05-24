package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/codegangsta/negroni"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

type App struct {
	DB     *sqlx.DB
	Router *mux.Router
	Cache  Cache
}

// Initialize creates the DB connection and prepares all the routes.
func (a *App) Initialize(cache Cache, db *sqlx.DB) {
	a.Cache = cache
	a.DB = db
	a.Router = mux.NewRouter()
	a.initializeRoutes()
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/users", a.getUsers).Methods("GET")
	a.Router.HandleFunc("/user", a.createUser).Methods("POST")
	a.Router.HandleFunc("/user/{id:[0-9]+}", a.getUser).Methods("GET")
	a.Router.HandleFunc("/user/{id:[0-9]+}", a.updateUser).Methods("PUT")
	a.Router.HandleFunc("/user/{id:[0-9]+}", a.deleteUser).Methods("DELETE")

}

// Run initializes the server and setup routes
func (a *App) Run(addr string) {
	n := negroni.Classic()
	n.UseHandler(a.Router)
	log.Fatal(http.ListenAndServe(addr, n))
}

// respondWithError handles the creation and sending error response to http
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

// respondWithJSON converts and send message response to http
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// getUser fetches a single user (user id is supplied). It first check the cache if the user exists.
func (a *App) getUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalide product ID")
		return
	}

	if value, err := a.Cache.getValue(id); err == nil && len(value) != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(value))
		return
	}

	user := User{ID: id}
	if err := user.get(a.DB); err != nil {
		switch err {
		case sql.ErrNoRows:
			respondWithError(w, http.StatusNotFound, "User not found")
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	response, _ := json.Marshal(user)
	// save the user to cache
	if err := a.Cache.setValue(user.ID, response); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func (a *App) getUsers(w http.ResponseWriter, r *http.Request) {
	count, _ := strconv.Atoi(r.FormValue("count")) // count value is passed to this method
	start, _ := strconv.Atoi(r.FormValue("start")) // start value is passed to this method

	if count > 10 || count < 1 {
		count = 10
	}
	if start < 0 {
		start = 0
	}

	// limit the list starting 'start' to start + count
	users, err := list(a.DB, start, count)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, users)
}

func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	var user User
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&user); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	// get sequence from Postgres
	a.DB.Get(&user.ID, "SELECT nextval('user_id_seq')")

	JSONByte, _ := json.Marshal(user)
	if err := a.Cache.setValue(user.ID, string(JSONByte)); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := a.Cache.enqueueValue(createUsersQueue, user.ID); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusCreated, user)
}

func (a *App) updateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid product ID")
		return
	}

	var user User
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&user); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()
	user.ID = id

	if err := user.update(a.DB); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, user)
}

func (a *App) deleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid User ID")
		return
	}

	user := User{ID: id}
	if err := user.delete(a.DB); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}
