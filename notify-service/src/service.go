package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/jabardigitalservice/jabarnotify-services/notify-service/src/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var userSession = make(map[string]map[string]interface{})

//Notification schema
type Notification struct {
	ID             primitive.ObjectID     `json:"_id,omitempty" bson:"_id,omitempty"`
	Body           string                 `json:"body,omitempty" bson:"body,omitempty" binding:"required"`
	Subject        string                 `json:"subject,omitempty" bson:"subject,omitempty"`
	Type           string                 `json:"type,omitempty" bson:"type,omitempty" binding:"required"`
	RecipientTotal int                    `json:"recipientTotal,omitempty" bson:"recipientTotal,omitempty"`
	Status         string                 `json:"status,omitempty" bson:"status,omitempty"`
	CreatedBy      map[string]interface{} `json:"createdBy,omitempty" bson:"createdBy,omitempty" binding:"required"`
	CreatedAt      time.Time              `json:"createdAt,omitempty" bson:"createdAt,omitempty" binding:"required"`
}

//NotificationRecipient schema
type NotificationRecipient struct {
	ID             primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	NotificationID primitive.ObjectID `json:"body,notificationId" bson:"notificationId,omitempty" binding:"required"`
	Name           string             `json:"name,omitempty" bson:"name,omitempty"`
	EmailAddress   string             `json:"emailAddress,omitempty" bson:"emailAddress,omitempty"`
	PhoneNumber    string             `json:"phoneNumber,omitempty" bson:"phoneNumber,omitempty"`
	Params         []string           `json:"params,omitempty" bson:"params,omitempty"`
	Status         string             `json:"status,omitempty" bson:"status,omitempty"`
	SendAt         time.Time          `json:"sendAt,omitempty" bson:"sendAt,omitempty"`
}

//MetaData struct
type MetaData struct {
	TotalCount  int `json:"totalCount"`
	TotalPage   int `json:"totalPage"`
	CurrentPage int `json:"currentPage"`
	PerPage     int `json:"perPage"`
}

//SiteService describe the Stats service
type SiteService interface {
	HealthCheck(ctx context.Context) (map[string]interface{}, error)
	GetNotification(ctx context.Context, page int, perPage int) ([]map[string]interface{}, *MetaData, error)
	DetailNotification(ctx context.Context, id string) (map[string]interface{}, error)
	GetNotificationSummary(ctx context.Context) (map[string]interface{}, error)
	CreateNotification(ctx context.Context,
		body string,
		subject string,
		typ string,
		recipients []*NotificationRecipient) (*Notification, error)
}

// NewSiteService returns a basic StatsService with all of the expected middlewares wired in.
func NewSiteService(logger log.Logger) SiteService {
	var svc SiteService
	svc = NewBasicService()
	svc = LoggingMiddleware(logger)(svc)
	return svc
}

// NewBasicService returns a naive, stateless implementation of StatsService.
func NewBasicService() SiteService {
	config, _ := utils.Initialize()
	return &basicService{
		DB: config.DB,
	}
}

type basicService struct {
	DB *mongo.Database
}

var (
	//ErrLoadNotif unable to find the requested team
	ErrLoadNotif = errors.New("error retriving notif")
	//ErrRequest unable to find the requested team
	ErrRequest = errors.New("error request")
	//ErrUnauthorized unable to verify token
	ErrUnauthorized = errors.New("Unauthorized")
	//ErrExpiredToken handle expiredToken
	ErrExpiredToken = errors.New("Token is expired")
	//ErrServiceUnavailable exception
	ErrServiceUnavailable = errors.New("Service Unavailable")
)

func getQueueName(typ string) string {
	queueName := ""

	switch typ {
	case "sms":
		queueName = "smsblast-queue"
	case "whatsapp":
		queueName = "wablast-queue"
	}

	return queueName
}

func pushNotifToPhoneNumber(queueName string, phoneNumber string, body string) {

	sess := session.New(&aws.Config{
		Region: aws.String(utils.GetEnv("AWS_DEFAULT_REGION")),
		Credentials: credentials.NewStaticCredentials(
			utils.GetEnv("AWS_ACCESS_KEY_ID"),
			utils.GetEnv("AWS_SECRET_ACCESS_KEY"), "",
		),
		MaxRetries: aws.Int(5),
	})

	svc := sqs.New(sess)

	confQueue, _ := svc.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})

	svc.SendMessage(&sqs.SendMessageInput{
		DelaySeconds: aws.Int64(10),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"PhoneNumber": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(utils.CleanPhoneNumber(phoneNumber)),
			},
		},
		MessageBody: aws.String(body),
		QueueUrl:    aws.String(*confQueue.QueueUrl),
	})
}

//GetNotif display notif list
func (s *basicService) GetNotification(ctx context.Context, page int, perPage int) ([]map[string]interface{}, *MetaData, error) {
	collection := s.DB.Collection("notifications")

	offset := (page - 1) * perPage
	sortStage := bson.D{{"$sort", bson.D{{"createdAt", -1}}}}
	skipStage := bson.D{{"$skip", offset}}
	limitStage := bson.D{{"$limit", perPage}}

	result, err := collection.Aggregate(ctx, mongo.Pipeline{sortStage, skipStage, limitStage})
	fmt.Println(result)
	if err != nil {
		panic(err)
	}

	var data []map[string]interface{}

	if err = result.All(ctx, &data); err != nil {
		panic(err)
	}

	total, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		panic(err)
	}

	metaData := &MetaData{
		TotalCount:  int(total),
		TotalPage:   utils.PageCount(int(total), int(perPage)),
		CurrentPage: int(page),
		PerPage:     int(perPage),
	}

	return data, metaData, nil
}

//DetailNotification display notif list
func (s *basicService) DetailNotification(ctx context.Context, id string) (map[string]interface{}, error) {
	_id, _ := primitive.ObjectIDFromHex(id)

	collection := s.DB.Collection("notifications")
	matchStage := bson.D{{"$match", bson.D{{"_id", _id}}}}
	lookupStage := bson.D{{"$lookup", bson.D{{"from", "notificationrecipients"}, {"localField", "_id"}, {"foreignField", "notificationId"}, {"as", "recipients"}}}}

	result, err := collection.Aggregate(ctx, mongo.Pipeline{matchStage, lookupStage})
	fmt.Println(result)
	if err != nil {
		panic(err)
	}

	var data []map[string]interface{}

	if err = result.All(ctx, &data); err != nil {
		panic(err)
	}

	return data[0], nil
}

//CreateNotif display notif list
func (s *basicService) CreateNotification(
	ctx context.Context,
	body string,
	subject string,
	typ string,
	recipients []*NotificationRecipient) (*Notification, error) {
	notification := &Notification{
		Body:           body,
		Subject:        subject,
		Type:           typ,
		RecipientTotal: len(recipients),
		Status:         "sent",
		CreatedBy:      userSession["user"],
		CreatedAt:      time.Now(),
	}

	collection := s.DB.Collection("notifications")
	insertResult, err := collection.InsertOne(context.TODO(), notification)

	if err != nil {
		return nil, err
	}

	queueName := getQueueName(typ)

	if queueName == "" || len(recipients) < 1 {
		return nil, ErrRequest
	}

	var notificationRecipients []interface{}
	for _, recipient := range recipients {
		recipient := &NotificationRecipient{
			NotificationID: insertResult.InsertedID.(primitive.ObjectID),
			Name:           recipient.Name,
			EmailAddress:   recipient.EmailAddress,
			PhoneNumber:    recipient.PhoneNumber,
			Status:         "sent",
		}
		notificationRecipients = append(notificationRecipients, recipient)
	}

	s.DB.Collection("notificationrecipients").InsertMany(context.TODO(), notificationRecipients)

	errs := make(chan error)
	go func() {
		c := make(chan os.Signal)
		for i, recipient := range recipients {
			messg := body
			messg = strings.ReplaceAll(messg, "{name}", recipient.Name)
			messg = strings.ReplaceAll(messg, "{phone_number}", recipient.PhoneNumber)
			messg = utils.ReplaceBodyParams(messg, recipients[i].Params)

			pushNotifToPhoneNumber(queueName, recipient.PhoneNumber, messg)
		}
		errs <- fmt.Errorf("%s", <-c)
	}()

	return notification, nil
}

// GetNotificationSummary func
func (s *basicService) GetNotificationSummary(ctx context.Context) (map[string]interface{}, error) {
	collection := s.DB.Collection("notifications")

	getCountDocuments := func(typ string, stat string) int64 {
		total, _ := collection.CountDocuments(ctx, bson.M{
			"type":   typ,
			"status": stat,
		})

		return total
	}

	getSummaryByNotifType := func(typ string) map[string]interface{} {
		summary := map[string]interface{}{
			"sent":   getCountDocuments(typ, "sent"),
			"failed": getCountDocuments(typ, "failed"),
		}
		return summary
	}

	data := map[string]interface{}{
		"sms":      getSummaryByNotifType("sms"),
		"whatsapp": getSummaryByNotifType("whatsapp"),
		"email":    getSummaryByNotifType("email"),
	}

	return data, nil
}

// HealthCheck func
func (s *basicService) HealthCheck(ctx context.Context) (map[string]interface{}, error) {
	res := s.DB.RunCommand(ctx, bson.D{{"ping", 1}}, options.RunCmd())

	if res.Err() != nil {
		return nil, ErrServiceUnavailable
	}

	result := map[string]interface{}{
		"status": "Service Available",
	}
	return result, nil
}
