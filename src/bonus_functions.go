package fission

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/functions/metadata"
	jwt "github.com/dgrijalva/jwt-go"
	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gentleman.v2/plugins/body"

	"cloud.google.com/go/firestore"
)

// bonus type
// unidirectional
// bidirectional
// realtime
// ontime

const (
	referralRecordsCollection = "referral_records"
	bonusHistoryCollection    = "bonus_history"
	dftBonusDays              = "3"
	dftBonusDuration          = "three_day"
)

// GOOGLE_CLOUD_PROJECT is automatically set by the Cloud Functions runtime.
var gcpProjectID = os.Getenv("GCP_PROJECT")
var bonusDaysEnv = os.Getenv("BonusDays")

var (
	berr      error
	bonusDays int64
	client    *firestore.Client
)

func init() {
	if gcpProjectID == "" {
		log.Fatal("PROJECT_ID environment variable must be set.")
	}
	if bonusDaysEnv == "" {
		bonusDaysEnv = dftBonusDays
	}
	bonusDays, _ = strconv.ParseInt(bonusDaysEnv, 10, 64)

	ctx := context.Background()

	client, berr = firestore.NewClient(ctx, gcpProjectID)
	if berr != nil {
		log.Fatalf("firestore.client: %v", berr)
	}
}

// FirestoreEvent is the payload of a Firestore event.
// Please refer to the docs for additional information
// regarding Firestore events.
type FirestoreEvent struct {
	OldValue FirestoreValue `json:"oldValue"`
	Value    FirestoreValue `json:"value"`
}

// FirestoreValue holds Firestore fields.
type FirestoreValue struct {
	CreateTime time.Time          `json:"createTime"`
	Fields     ReferralRecordData `json:"fields"`
	Name       string             `json:"name"`
	UpdateTime time.Time          `json:"updateTime"`
}

// ReferralRecordData represents a value from Firestore. The type definition depends on the
// format of your database.
type ReferralRecordData struct {
	CreatedAt struct {
		IntegerValue string `json:"integerValue"`
	} `json:"createdAt"`
	UpdatedAt struct {
		IntegerValue string `json:"integerValue"`
	} `json:"updatedAt"`

	Uid struct {
		StringValue string `json:"stringValue"`
	} `json:"uid"`
	ReferredByUid struct {
		StringValue string `json:"stringValue"`
	} `json:"referred_by_uid"`
	BonusType struct {
		StringValue string `json:"stringValue"`
	} `json:"bonus_type"`
	Level struct {
		IntegerValue string `json:"integerValue"`
	} `json:"level"`
	IsIntegratedPurchaseService struct {
		BooleanValue bool `json:"booleanValue"`
	} `json:"is_integrated_purchase_service"`
}

type (
	// ReferralRecordDoc ...
	ReferralRecordDoc struct {
		CreatedAt                   int64  `firestore:"createdAt" json:"createdAt"`
		UpdatedAt                   int64  `firestore:"updatedAt" json:"updatedAt"`
		Uid                         string `firestore:"uid" json:"uid"`
		ReferredByUid               string `firestore:"referred_by_uid" json:"referred_by_uid"`
		BonusType                   string `firestore:"bonus_type" json:"bonus_type"`
		Level                       int64  `firestore:"level" json:"level"`
		IsIntegratedPurchaseService bool   `firestore:"is_integrated_purchase_service" json:"is_integrated_purchase_service"`
	}

	// BonusItem ...
	BonusItem struct {
		CreatedAt  int64  `firestore:"createdAt" json:"createdAt"`
		StartedAt  int64  `firestore:"startedAt" json:"startedAt"`
		BonusType  string `firestore:"bonus_type" json:"bonus_type"`
		ExpireTime int64  `firestore:"expire_time" json:"expire_time"`
	}
	// BonusHistory ...
	BonusHistory struct {
		CreatedAt int64       `firestore:"createdAt" json:"createdAt"`
		UpdatedAt int64       `firestore:"updatedAt" json:"updatedAt"`
		Uid       string      `firestore:"uid" json:"uid"`
		ExpiredAt int64       `firestore:"expired_at" json:"expired_at"`
		Bonuses   []BonusItem `firestore:"bonuses" json:"bonuses"`
	}

	// Identity identity in credential payload
	Identity struct {
		AppVersion  string `json:"app_version,omitempty"`
		AppPlatform string `json:"app_platform,omitempty"`
	}

	// JwtCustomClaims custome Claims for credential payload
	JwtCustomClaims struct {
		Identity `json:"identity,omitempty"`
		KID      string `json:"kid,omitempty"`
		jwt.StandardClaims
	}

	// EntitlementsPutRequest ...
	EntitlementsPutRequest struct {
		AppUserID     string `json:"app_user_id,omitempty"`
		EntitlementID string `json:"entitlement_id,omitempty"`
		Duration      string `json:"duration,omitempty"`
	}
)

// HelloBonus is triggered by a change to a Firestore document.
func HelloBonus(ctx context.Context, e FirestoreEvent) error {
	meta, err := metadata.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("metadata.FromContext: %v", err)
	}
	log.Printf("Function triggered by change to: %v", meta.Resource)
	log.Printf("%v", e)

	fullPath := strings.Split(e.Value.Name, "/documents/")[1]
	pathParts := strings.Split(fullPath, "/")
	collection := pathParts[0]
	doc := strings.Join(pathParts[1:], "/")
	log.Printf("full path=%v, path parts=%v, collection=%v, doc=%v\n", fullPath, pathParts, collection, doc)

	createdAtStr := e.Value.Fields.CreatedAt.IntegerValue
	createdAt, _ := strconv.ParseInt(createdAtStr, 10, 64)
	uid := e.Value.Fields.Uid.StringValue
	referredByUid := e.Value.Fields.ReferredByUid.StringValue
	bonusType := e.Value.Fields.BonusType.StringValue
	isIntegratedPurchaseService := e.OldValue.Fields.IsIntegratedPurchaseService.BooleanValue

	// todo: level trigger
	levelValue := e.Value.Fields.Level.IntegerValue
	level, _ := strconv.ParseInt(levelValue, 10, 64)
	log.Println("level value=", level)

	// referralRecord, _ := client.Collection(referralCollection).Doc(uid).Get(ctx)
	// if referralRecord.Exists() {
	// 	log.Printf("referral record already existed, record=%v\n", referralRecord)
	// 	fmt.Fprintf(w, dftErrResponse, "UID Exists")
	// 	return
	// }

	bts := strings.Split(bonusType, "_")
	direction := bts[0]
	timeCond := bts[1]
	log.Printf("direction=%v, timecond=%v\n", direction, timeCond)

	switch direction {
	case "unidirectional":
		log.Println("unidirectional")
		bonusUser(bonusType, uid, timeCond, createdAt, isIntegratedPurchaseService)
	case "bidirectional":
		log.Println("bidirectional")
		bonusUser(bonusType, uid, timeCond, createdAt, isIntegratedPurchaseService)
		bonusUser(bonusType, referredByUid, timeCond, createdAt, isIntegratedPurchaseService)
	}

	return nil
}

func bonusUser(bonusType, uid, timeCond string, createdAt int64, isIntegratedPurchaseService bool) error {
	ctx := context.Background()
	dsnap, err := client.Collection(bonusHistoryCollection).Doc(uid).Get(ctx)
	if err != nil {
		log.Printf("get archive update error=%v\n", err)
	}

	var bhDoc BonusHistory
	dsnap.DataTo(&bhDoc)
	log.Printf("Document data: %#v\n", bhDoc)

	item := BonusItem{
		CreatedAt:  createdAt,
		StartedAt:  time.Now().Unix(),
		BonusType:  bonusType,
		ExpireTime: bonusDays,
	}
	// append bonus record
	bhDoc.Bonuses = append(bhDoc.Bonuses, item)
	// todo, update expired_at
	client.Collection(bonusHistoryCollection).Doc(uid).Set(ctx, bhDoc)

	if isIntegratedPurchaseService {
		bonusUserByPurchaseService(uid, dftBonusDuration)
	}
	return nil
}

// genToken generate sign token
func genToken() (tokenStr string, err error) {
	sharedSecret := os.Getenv("WoolongSharedSecret")

	appVersion := os.Getenv("WoolongAppVersion")
	appPlatform := os.Getenv("WoolongAppPlatform")

	// change expire time in seconds
	// expireInSecs := os.Getenv("woolong.expireInMilSecs")

	// Create the Claims
	claims := JwtCustomClaims{
		Identity: Identity{
			AppVersion:  appVersion,
			AppPlatform: appPlatform,
		},
		KID: os.Getenv("WoolongKid"),
		StandardClaims: jwt.StandardClaims{
			Issuer:    os.Getenv("WoolongIssuer"),
			ExpiresAt: time.Now().Unix() + 86400,
			Audience:  os.Getenv("WoolongAudience"),
		},
	}

	signKey := []byte(sharedSecret)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err = token.SignedString(signKey)
	fmt.Printf("token=%v, err=%+v\n", tokenStr, err)
	return tokenStr, err
}

func bonusUserByPurchaseService(userID string, duration string) (resp *gentleman.Response, err error) {
	token, err := genToken()
	if err != nil {
		log.Println("gen woolong token err", err)
		return
	}

	requestData := EntitlementsPutRequest{
		AppUserID:     userID,
		EntitlementID: os.Getenv("WoolongEntitlementId"),
		Duration:      duration,
	}
	url := os.Getenv("WoolongEntitlementUrl")

	cli := gentleman.New()
	cli.URL(url)

	// Create a new request based on the current client
	req := cli.Request()
	// Set headers
	req.SetHeader("Content-Type", "application/json")
	req.SetHeader("Accept", "application/json")
	req.SetHeader("Authorization", "Bearer "+token)

	req.Method("PUT")
	req.Use(body.JSON(requestData))

	// Perform the request
	resp, err = req.Send()
	if err != nil {
		fmt.Printf("Request error: %s\n", err)
		return
	}
	log.Printf("resp %T %+v", resp, resp)
	if !resp.Ok {
		fmt.Printf("Invalid server response: %d\n", resp.StatusCode)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Body: %s", resp.String())

	return
}
