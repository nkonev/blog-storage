package mongo_lock

import (
	"context"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
		Logger.Panicf("Error during creating bson from \"%v\": %v", data, err)
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
		Logger.Panicf("Error during creating unique index: %v", commandResult.Err())
	}
}

func getUpdateDoc(p bson.M) bson.M {
	update := bson.M{"$set": p}
	return update
}

func (ml *mongoLock) AcquireLock() {
	ensureIndex(ml.mongoClient, ml.lockCollection)
	database := utils.GetMongoDatabase(ml.mongoClient)

	var upsert = true
	duration, _ := time.ParseDuration("1s")

	for {
		result, err := database.Collection(ml.lockCollection).UpdateOne(context.TODO(), ml.idDoc, getUpdateDoc(bson.M{"lastAcquired": time.Now()}), &options.UpdateOptions{Upsert: &upsert})
		if err != nil {
			Logger.Panicf("Error during acquiring lock: %v", err)
		} else {
			if result.UpsertedID != nil {
				Logger.Infof("Lock has been acquired")
				break
			} else {
				Logger.Infof("Lock has n' t been acquired - waiting %v", duration)
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
		Logger.Panicf("Error during releasing lock: %v", err)
	}
	Logger.Infof("Lock successfully released")
}
