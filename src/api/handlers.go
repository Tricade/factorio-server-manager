package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/sessions"
)

const readHttpBodyError = "Could not read the Request Body."
const maximumJSONRequestBodyBytes int64 = 1 * 1024 * 1024
const multipartRequestOverheadBytes int64 = 1 * 1024 * 1024

type JSONResponseFileInput struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data"`
	Error     string      `json:"error"`
	ErrorKeys []int       `json:"errorkeys"`
}

func WriteResponse(w http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error writing response: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func ReadRequestBody(w http.ResponseWriter, r *http.Request) (body []byte, resp interface{}, err error) {
	if r.Body == nil {
		resp = fmt.Sprintf("%s: no request body", readHttpBodyError)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		err = errors.New("no request body")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maximumJSONRequestBodyBytes)
	body, err = io.ReadAll(r.Body)
	if err != nil {
		body = nil
		resp = fmt.Sprintf("%s: %s", readHttpBodyError, err)
		log.Println(resp)
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	return
}

func ReadSessionStore(w http.ResponseWriter, r *http.Request, name string) (session *sessions.Session, resp interface{}, err error) {
	session, err = sessionStore.Get(r, name)
	if err != nil {
		resp = fmt.Sprintf("Error reading session cookie [%s]: %s", name, err)
		log.Println(resp)
		if session != nil {
			session.Options.MaxAge = -1
			err2 := session.Save(r, w)
			if err2 != nil {
				log.Printf("Error deleting session cookie: %s", err2)
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
	return
}

func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) (resp interface{}, err error) {
	err = session.Save(r, w)
	if err != nil {
		resp = fmt.Sprintf("Error saving session cookie: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
	}
	return
}
