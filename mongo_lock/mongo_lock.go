package mongo_lock

import (
	"context"
	"github.com/labstack/gommon/log"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/nkonev/blog-store/utils"
	"time"
)

type mongoLock struct {
	mongoClient    *mongo.Client
	lockCollection string
	idDoc          bson.D
}

func GetIdDoc() bson.D {
	return bson.D{{"_id", 42}}
}

func NewMongoLock(mongoClient *mongo.Client, lockCollection string) *mongoLock {
	return &mongoLock{mongoClient: mongoClient, lockCollection: lockCollection, idDoc: GetIdDoc()}
}

func createBson(data string) bson.D {
	bdoc := bson.D{}
	err := bson.UnmarshalExtJSON([]byte(data), true, &bdoc)
	if err != nil {
		log.Panicf("Error during creating bson from \"%v\": %v", data, err)
	}
	return bdoc
}

func ensureIndex(client *mongo.Client, lockCollection string) {
	database := utils.GetMongoDatabase(client)

	commandResult := database.RunCommand(context.TODO(), createBson(`{
        "createIndexes": "`+lockCollection+`",
        "indexes": [
            {
                "key": {
                    "id": 1
                },
                "name": "unique_id",
	        	"unique": true
            }
        ]
}`))
	if commandResult.Err() != nil {
		log.Panicf("Error during creating unique index: %v", commandResult.Err())
	}
}

func (ml *mongoLock) AcquireLock() {
	ensureIndex(ml.mongoClient, ml.lockCollection)
	database := utils.GetMongoDatabase(ml.mongoClient)

	var upsert = true
	duration, _ := time.ParseDuration("1s")

	for {
		result, err := database.Collection(ml.lockCollection).UpdateOne(context.TODO(), ml.idDoc, bson.D{}, &options.UpdateOptions{Upsert: &upsert})
		if err != nil {
			log.Panicf("Error during acquiring lock: %v", err)
		} else {
			if result.UpsertedID != nil {
				log.Infof("Lock has been acquired")
				break
			} else {
				log.Infof("Lock has n' t been acquired - waiting %v", duration)
				time.Sleep(duration)
				continue
			}
		}
	}
}

func (ml *mongoLock) ReleaseLock() {
	database := utils.GetMongoDatabase(ml.mongoClient)
	_, err := database.Collection(ml.lockCollection).DeleteOne(context.TODO(), ml.idDoc)
	if err != nil {
		log.Panicf("Error during releasing lock: %v", err)
	}
	log.Infof("Lock successfully released")
}
