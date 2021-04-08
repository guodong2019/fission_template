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

// ToDo:
// firestore write trigger;
// create update delete trigger bonus action;
// avoid repeated trigger

// bonus direction:
//     referredbyuid    uid
//            2          1
// bidirectional: 3 = 1 + 2
// 1. unidirectional_uid
// 2. unidirectional_referredbyuid
// 3. bidirectional (default)

// bonus conditon:
// 1. immediately
// 2. ontime

// bonus type
// 1. daily: 86400
// 2. three_day: 86400 * 3
// 3. weekly:
// 4. monthly:
// 5. six_month:
// 6. yearly:
// 7. lifetime:
// ...

const (
	referralRecordsCollection = "referral_records"
	bonusHistoryCollection    = "bonus_history"
)

// GOOGLE_CLOUD_PROJECT is automatically set by the Cloud Functions runtime.
var gcpProjectID = os.Getenv("GCP_PROJECT")

var (
	berr   error
	client *firestore.Client
)

var bonusTypeTimeRef = map[string]int64{
	"1": 86400,
	"2": 86400 * 3,
	"3": 86400 * 7,
	"4": 86400 * 30,
	"5": 86400 * 30 * 6,
	"6": 86400 * 365,
	"7": 86400 * 365 * 10,
}
var bonusTypeDurationRef = map[string]string{
	"1": "daily",
	"2": "three_day",
	"3": "weekly",
	"4": "monthly",
	"5": "six_month",
	"6": "yearly",
	"7": "lifetime",
}

func init() {
	if gcpProjectID == "" {
		log.Fatal("PROJECT_ID environment variable must be set.")
	}

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
	BonusCondition struct {
		IntegerValue string `json:"integerValue"`
	} `json:"bonus_condition"`
	BonusDirection struct {
		IntegerValue string `json:"integerValue"`
	} `json:"bonus_direction"`
	BonusType struct {
		IntegerValue string `json:"integerValue"`
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
		CreatedAt                   int64  `firestore:"created_at" json:"created_at"`
		UpdatedAt                   int64  `firestore:"updated_at" json:"updated_at"`
		Uid                         string `firestore:"uid" json:"uid"`
		ReferredByUid               string `firestore:"referred_by_uid" json:"referred_by_uid"`
		BonusCondition              int    `firestore:"bonus_condition" json:"bonus_condition"`
		BonusDirection              int    `firestore:"bonus_direction" json:"bonus_direction"`
		BonusType                   int    `firestore:"bonus_type" json:"bonus_type"`
		Level                       int64  `firestore:"level" json:"level"`
		IsIntegratedPurchaseService bool   `firestore:"is_integrated_purchase_service" json:"is_integrated_purchase_service"`
	}

	// BonusItem ...
	BonusItem struct {
		ReferraledAt int64  `firestore:"referraled_at" json:"referraled_at"`
		StartedAt    int64  `firestore:"started_at" json:"started_at"`
		BonusType    string `firestore:"bonus_type" json:"bonus_type"`
		ExpireTime   int64  `firestore:"expire_time" json:"expire_time"`
	}
	// BonusHistory ...
	BonusHistory struct {
		CreatedAt int64       `firestore:"created_at" json:"created_at"`
		UpdatedAt int64       `firestore:"updated_at" json:"updated_at"`
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
	bonusCondition := e.Value.Fields.BonusCondition.IntegerValue
	bonusDirection := e.Value.Fields.BonusDirection.IntegerValue
	bonusType := e.Value.Fields.BonusType.IntegerValue
	isIntegratedPurchaseService := e.OldValue.Fields.IsIntegratedPurchaseService.BooleanValue

	// todo: level trigger
	levelValue := e.Value.Fields.Level.IntegerValue
	level, _ := strconv.ParseInt(levelValue, 10, 64)
	log.Println("level value=", level)

	switch bonusDirection {
	case "1":
		log.Println("unidirectional uid")
		bonusUser(uid, bonusCondition, bonusType, createdAt, isIntegratedPurchaseService)
	case "2":
		log.Println("unidirectional referredbyuid")
		bonusUser(referredByUid, bonusCondition, bonusType, createdAt, isIntegratedPurchaseService)
	case "3":
		log.Println("bidirectional, both uid and referredbyuid")
		bonusUser(uid, bonusCondition, bonusType, createdAt, isIntegratedPurchaseService)
		bonusUser(referredByUid, bonusCondition, bonusType, createdAt, isIntegratedPurchaseService)
	}

	return nil
}

func bonusUser(userId string, bonusCondition, bonusType string, referraledAt int64, isIntegratedPurchaseService bool) error {
	// bonusCondition
	// todo, support condition

	ctx := context.Background()

	dsnap, err := client.Collection(bonusHistoryCollection).Doc(userId).Get(ctx)
	if err != nil {
		log.Printf("get bonus doc error=%v\n", err)
	}

	var bhDoc BonusHistory

	now := time.Now().Unix()
	bonusTypeTime := bonusTypeTimeRef[bonusType]
	item := BonusItem{
		ReferraledAt: referraledAt,
		StartedAt:    now,
		BonusType:    bonusType,
		ExpireTime:   bonusTypeTime,
	}

	if !dsnap.Exists() {
		bhDoc.CreatedAt = now
		bhDoc.UpdatedAt = now
		bhDoc.Uid = userId
		bhDoc.ExpiredAt = bonusTypeTime
	} else {
		dsnap.DataTo(&bhDoc)
		log.Printf("Document raw data: %#v\n", bhDoc)
		bhDoc.UpdatedAt = now
		bhDoc.ExpiredAt = bhDoc.ExpiredAt + bonusTypeTime
	}

	// append bonus record
	bhDoc.Bonuses = append(bhDoc.Bonuses, item)
	log.Printf("Document after data: %#v\n", bhDoc)

	client.Collection(bonusHistoryCollection).Doc(userId).Set(ctx, bhDoc)

	if isIntegratedPurchaseService {
		bonusUserByPurchaseService(userId, bonusTypeDurationRef[bonusType])
	}
	return nil
}

// genToken generate sign token
func genToken() (tokenStr string, err error) {
	sharedSecret := os.Getenv("WoolongSharedSecret")
	appVersion := os.Getenv("WoolongAppVersion")
	appPlatform := os.Getenv("WoolongAppPlatform")

	// Create the Claims
	claims := JwtCustomClaims{
		Identity: Identity{
			AppVersion:  appVersion,
			AppPlatform: appPlatform,
		},
		KID: os.Getenv("WoolongKid"),
		StandardClaims: jwt.StandardClaims{
			Issuer:    os.Getenv("WoolongIssuer"),
			ExpiresAt: time.Now().Unix() + 86400*2,
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
