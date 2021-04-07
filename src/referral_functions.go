package fission

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
)

const (
	referralCollection = "referral_records"
	authorization      = "Authorization"
)

var (
	app        *firebase.App
	authClient *auth.Client
	dbClient   *firestore.Client
	err        error
)

// default response
var (
	dftOkResponse  = `{"status": "ok", "message": "succeed."}`
	dftErrResponse = `{"status": "error", "message": "error=%v!"}`
)

// GOOGLE_CLOUD_PROJECT is automatically set by the Cloud Functions runtime.
var projectID = os.Getenv("GCP_PROJECT")

type (
	requestPayload struct {
		ReferredByUid               string `json:"referred_by_uid"`
		BonusType                   string `json:"bonus_type"`
		IsIntegratedPurchaseService bool   `json:"is_integrated_purchase_service"`
	}

	ReferralRecord struct {
		CreatedAt                   int64  `firestore:"createdAt" json:"createdAt"`
		UpdatedAt                   int64  `firestore:"updatedAt" json:"updatedAt"`
		Uid                         string `firestore:"uid" json:"uid"`
		ReferredByUid               string `firestore:"referred_by_uid" json:"referred_by_uid"`
		BonusType                   string `firestore:"bonus_type" json:"bonus_type"`
		Level                       int64  `firestore:"level" json:"level"`
		IsIntegratedPurchaseService bool   `firestore:"is_integrated_purchase_service" json:"is_integrated_purchase_service"`
	}
)

func init() {
	if projectID == "" {
		log.Fatal("PROJECT_ID environment variable must be set.")
	}

	ctx := context.Background()
	app, err = firebase.NewApp(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to create app client: %v", err)
	}

	authClient, err = app.Auth(ctx)
	if err != nil {
		log.Fatalf("Failed to create auth client: %v", err)
	}

	dbClient, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("firestore.client: %v", err)
	}
}

// HelloReferral is a simple HTTP handler that addresses HTTP requests to the /hello endpoint
func HelloReferral(w http.ResponseWriter, r *http.Request) {
	// authorizationToken := r.Header.Get(authorization)
	// if authorizationToken == "" {
	// 	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	// 	return
	// }
	// log.Println(authorizationToken)
	// var err error
	ctx := context.Background()
	// var token *auth.Token
	// token, err = authClient.VerifyIDToken(ctx, authorizationToken)
	// if err != nil {
	// 	log.Println(err.Error())
	// 	http.Error(w, err.Error(), http.StatusUnauthorized)
	// 	return
	// }

	// uid := token.UID
	uid := "test_uid_abcd_efg"
	if uid == "" {
		http.Error(w, fmt.Sprintf("invalid %s uid", uid), http.StatusBadRequest)
		return
	}

	var payload requestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("decode request body error=%v\n", err)
		fmt.Fprintf(w, dftErrResponse, "Invalid Request Payload")
		return
	}
	log.Printf("request payload=%v\n", payload)
	defer r.Body.Close()

	referralRecord, _ := dbClient.Collection(referralCollection).Doc(uid).Get(ctx)
	if referralRecord.Exists() {
		log.Printf("referral record already existed, record=%v\n", referralRecord)
		fmt.Fprintf(w, dftErrResponse, "UID Exists")
		return
	}

	record := ReferralRecord{
		CreatedAt:                   time.Now().Unix(),
		UpdatedAt:                   time.Now().Unix(),
		Uid:                         uid,
		ReferredByUid:               payload.ReferredByUid,
		BonusType:                   payload.BonusType,
		IsIntegratedPurchaseService: payload.IsIntegratedPurchaseService,
	}
	_, err = dbClient.Collection(referralCollection).Doc(uid).Set(ctx, record)
	if err != nil {
		fmt.Fprintf(w, dftErrResponse, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(dftOkResponse))
}
