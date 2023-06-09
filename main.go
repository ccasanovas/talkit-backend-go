package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"github.com/gorilla/mux"
	"errors"
	"time"
	firebase "firebase.google.com/go"
	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type FirestoreEvent struct {
	OldValue   FirestoreValue `json:"oldValue"`
	Value      FirestoreValue `json:"value"`
	UpdateMask struct {
		FieldPaths []string `json:"fieldPaths"`
	} `json:"updateMask"`
}

type FirestoreValue struct {
	CreateTime time.Time `json:"createTime"`
	Name       string    `json:"name"`
	UpdateTime time.Time `json:"updateTime"`
	Fields     UsersFieldsType      `json:"fields"`
}

// UsersType represents the Users collection in the database
type UsersType []map[string]interface{}

// UsersFieldsType defines the structure of the fields in an Users from the Users collection.
type UsersFieldsType struct {
	ID          string  `firestore:"uid"`
	Name        string  `firestore:"displayName"`
	Price       float64 `firestore:"price"`
	Type        string  `firestore:"type"`
	Year        string  `firestore:"year"`
	Image       string  `firestore:"image"`
	Description string  `firestore:"description"`
	Slug        string  `firestore:"slug"`
}

// DeleteType represents the body expected structure of a delete http call
type DeleteType struct {
	ID string `json:"id"`
}


func main() {
	// This example uses gorilla/mux as the router, whereas cloud functions are simple Http handlers
	router := mux.NewRouter()
	router.HandleFunc("/users", UsersAPI)
	//router.HandleFunc("/chats", UsersAPI)
	//router.HandleFunc("/messages", UsersAPI)
	//router.HandleFunc("/groups", UsersAPI)
	//router.HandleFunc("/talks", UsersAPI)
	router.HandleFunc("/suscriptions", SuscriptionsAPI)

	srv := &http.Server{
		Handler:      router,
		Addr:         "0.0.0.0:8000",
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}

	log.Println("Running server on http://localhost:8000")
	log.Fatal(srv.ListenAndServe())
}

// UsersAPI is an HTTP Cloud Function with a request parameter.
func UsersAPI(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	conf := &firebase.Config{ProjectID: "talkit-199f9"}

	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		log.Printf("error initializing app: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	client, err := app.Firestore(ctx)
	if err != nil {
		log.Printf("Firestore init: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer client.Close()



	// Set CORS headers for the preflight request
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Set CORS headers for the main request.
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch method := r.Method; method {
	case http.MethodGet:
		getUsers(ctx, client, w, r)
	case http.MethodPost:
		authorizeRequest(w, app, r)
		setUsers(ctx, client, w, r)
	case http.MethodDelete:
		authorizeRequest(w, app, r)
		deleteUsers(ctx, client, w, r)
	case http.MethodPut:
		authorizeRequest(w, app, r)
		updateUsers(ctx, client, w, r)
	default:
		http.Error(w, "UNSUPPORTED METHOD", http.StatusNotFound)
	}

}

func authorizeRequest(w http.ResponseWriter, app *firebase.App, r *http.Request ) {
	ctx := context.Background()

	auth, authErr := app.Auth(ctx)
	if authErr != nil {
		log.Fatalf("error getting Auth client: %v\n", authErr)
	}

	// Read Auth Jwt to access to this api
	token, authErr := auth.VerifyIDToken(ctx, r.Header.Get("Authorization"))

	if authErr != nil {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "FORBIDDEN",
			"statusCode": 403,
			"data": nil,
			"message": "You are trying to access to this api with malformed or unhauthenticated user",
		})
		return
	}

	log.Printf("Verified ID token: %v\n", token)

}


// Handles the rollback to a previous document
func handleRollback(ctx context.Context, e FirestoreEvent) error {
	return errors.New("Should have rolled back to a previous version")
}

// The function that runs with the cloud function itself
func HandleUserCreate(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request, e FirestoreEvent) error {
	// This is the data that's in the database itself
	newFields := e.Value.Fields

	//Set time for createdAt and ExpireAt
	location,_ := time.LoadLocation("America/Buenos_Aires")

	// this should give you time in location
	t := time.Now().In(location)


	//
	suscription := map[string]interface{}{
		"expired":   false,
		"suscriptionType": "free-trial",
		"cost":    0,
		"expireAt": t.AddDate(0, 0, 7 * 12).Format(http.TimeFormat),
		"createdAt": t.Format(http.TimeFormat),
	}

	jsonSuscription, err := json.Marshal(suscription)

	_, err = client.Collection("Suscriptions").Doc(newFields.ID).Create(ctx, &jsonSuscription)
	if err != nil {
		log.Printf("Collection update failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	w.WriteHeader(http.StatusCreated)

	return nil
}

func getUsers(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	var Users UsersType
	uid := r.URL.Query().Get("uid")
	iter := client.Collection("user").Where("uid", "==", uid).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Iteration over documents failed %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		Users = append(Users, doc.Data())
	}

	if Users != nil {
		json.NewEncoder(w).Encode(Users[0])
	} else {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "NOT_FOUND",
			"statusCode": 404,
			"data": nil,
			"message": "User uid not found",
		})
	}
}

func setUsers(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	var newUsers UsersFieldsType

	err = json.Unmarshal(body, &newUsers)
	if err != nil {
		log.Printf("Unmarshalling json failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_, err = client.Collection("Users").Doc(newUsers.ID).Create(ctx, &newUsers)
	if err != nil {
		log.Printf("Collection update failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func deleteUsers(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading request body failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var Body DeleteType

	err = json.Unmarshal(body, &Body)
	if err != nil {
		log.Printf("Unmarshalling json failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = client.Collection("Users").Doc(Body.ID).Delete(ctx)
	if err != nil {
		log.Printf("Document deletion failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func updateUsers(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading request body failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var Body UsersFieldsType

	err = json.Unmarshal(body, &Body)
	if err != nil {
		log.Printf("Unmarshalling json failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = client.Collection("Users").Doc(Body.ID).Set(ctx, &Body)
	if err != nil {
		log.Printf("Document update failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// UsersAPI is an HTTP Cloud Function with a request parameter.
func SuscriptionsAPI(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	conf := &firebase.Config{ProjectID: "talkit-199f9"}

	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		log.Printf("error initializing app: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	client, err := app.Firestore(ctx)
	if err != nil {
		log.Printf("Firestore init: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer client.Close()



	// Set CORS headers for the preflight request
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Set CORS headers for the main request.
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch method := r.Method; method {
	case http.MethodGet:
		getSuscriptions(ctx, client, w, r)
	case http.MethodPost:
		authorizeRequest(w, app, r)
		setSuscriptions(ctx, client, w, r)
	case http.MethodDelete:
		authorizeRequest(w, app, r)
		deleteSuscriptions(ctx, client, w, r)
	case http.MethodPut:
		authorizeRequest(w, app, r)
		updateSuscriptions(ctx, client, w, r)
	default:
		http.Error(w, "UNSUPPORTED METHOD", http.StatusNotFound)
	}

}


func getSuscriptions(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	var Users UsersType
	uid := r.URL.Query().Get("uid")
	iter := client.Collection("user").Where("uid", "==", uid).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Iteration over documents failed %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		Users = append(Users, doc.Data())
	}

	if Users != nil {
		json.NewEncoder(w).Encode(Users[0])
	} else {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "NOT_FOUND",
			"statusCode": 404,
			"data": nil,
			"message": "User uid not found",
		})
	}
}

func setSuscriptions(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	var newUsers UsersFieldsType

	err = json.Unmarshal(body, &newUsers)
	if err != nil {
		log.Printf("Unmarshalling json failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_, err = client.Collection("Suscriptions").Doc(newUsers.ID).Create(ctx, &newUsers)
	if err != nil {
		log.Printf("Collection update failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func deleteSuscriptions(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading request body failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var Body DeleteType

	err = json.Unmarshal(body, &Body)
	if err != nil {
		log.Printf("Unmarshalling json failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = client.Collection("Suscriptions").Doc(Body.ID).Delete(ctx)
	if err != nil {
		log.Printf("Document deletion failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func updateSuscriptions(ctx context.Context, client *firestore.Client, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Reading request body failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var Body UsersFieldsType

	err = json.Unmarshal(body, &Body)
	if err != nil {
		log.Printf("Unmarshalling json failed %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = client.Collection("Suscriptions").Doc(Body.ID).Set(ctx, &Body)
	if err != nil {
		log.Printf("Document update failed %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

