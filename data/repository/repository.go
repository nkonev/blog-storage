package repository

import (
	"context"
	"errors"
	"github.com/nkonev/blog-storage/logger"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const Id = "_id"
const filename = "filename"
const published = "published"
const userId = "userid"

const collectionLimits = "limits"
const collectionGlobalObjects = "global_objects"

// https://vkt.sh/go-mongodb-driver-cookbook/
type UserFileDto struct {
	Id        primitive.ObjectID `bson:"_id,omitempty"` // mongo document id equal to minio object jd
	Filename  string
	Published bool
	UserId    int64
}

type UserFileRepository struct {
	mongo *mongo.Client
}

func NewUserFileRepository(mongo *mongo.Client) *UserFileRepository {
	return &UserFileRepository{mongo: mongo}
}

type LimitsRepository struct {
	mongo *mongo.Client
}

func NewLimitsRepository(mongo *mongo.Client) *LimitsRepository {
	return &LimitsRepository{mongo: mongo}
}

func ToFileMongoDto(c *mongo.Cursor) (*UserFileDto, error) {
	var elem UserFileDto
	err := c.Decode(&elem)
	if err != nil {
		return nil, err
	}
	return &elem, nil
}

func GetIdDoc(objectId string) (*bson.D, error) {
	ids, e := primitive.ObjectIDFromHex(objectId)
	if e != nil {
		return nil, e
	}
	ds := bson.D{{Id, ids}}
	return &ds, nil
}

func GetUpdateDoc(p bson.M) bson.M {
	update := bson.M{"$set": p}
	return update
}

func (r *UserFileRepository) GetUserIdByGlobalId(objectId string) (int, error) {
	ids, e := primitive.ObjectIDFromHex(objectId)
	if e != nil {
		return 0, e
	}
	database := utils.GetMongoDatabase(r.mongo)

	ms := bson.M{Id: ids}
	one := database.Collection(collectionGlobalObjects).FindOne(context.TODO(), ms)
	if one.Err() != nil {
		if one.Err() != mongo.ErrNoDocuments {
			logger.Logger.Errorf("Error during get user id by global id %v", objectId)
		} else {
			logger.Logger.Infof("No documents found by global id %v", objectId)
		}
		return 0, one.Err()
	}
	var elem UserFileDto
	if err := one.Decode(&elem); err != nil {
		return 0, err
	}
	return int(elem.UserId), nil
}

func (r *UserFileRepository) InsertMetaInfoToMongo(filename string, userId int) (*string, error) {
	database := utils.GetMongoDatabase(r.mongo)

	inserted, err := database.Collection(collectionGlobalObjects).InsertOne(context.TODO(), UserFileDto{Filename: filename, Published: false, UserId: int64(userId)})
	if err != nil {
		Logger.Errorf("Error during create mongo metadata document: %v", err)
		return nil, err
	}
	idMongo := inserted.InsertedID.(primitive.ObjectID).Hex()
	return &idMongo, nil
}

func (r *UserFileRepository) GetMetainfoFromMongo(objectId string) (*UserFileDto, error) {
	database := utils.GetMongoDatabase(r.mongo)
	var userFilesCollection *mongo.Collection = database.Collection(collectionGlobalObjects)

	ds, err := GetIdDoc(objectId)
	if err != nil {
		logger.Logger.Errorf("Error during creating id document %v", objectId)
		return nil, err
	}

	one := userFilesCollection.FindOne(context.TODO(), ds)
	if one == nil {
		return nil, errors.New("Unexpected nil by id " + objectId)
	}
	if one.Err() != nil {
		logger.Logger.Errorf("Error during querying record from mongo by key %v", objectId)
		return nil, one.Err()
	}

	var elem UserFileDto
	if err := one.Decode(&elem); err != nil {
		if err == mongo.ErrNoDocuments {
			logger.Logger.Errorf("No documents found by key %v", objectId)
		}
		return nil, err
	}
	return &elem, nil
}

func (r *UserFileRepository) RenameUserFile(objId string, newname string) error {
	database := utils.GetMongoDatabase(r.mongo)
	var userFilesCollection *mongo.Collection = database.Collection(collectionGlobalObjects)

	findDocument, err := GetIdDoc(objId)
	if err != nil {
		return err
	}
	updateDocument := GetUpdateDoc(primitive.M{filename: newname})

	one := userFilesCollection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return one.Err()
	}
	return nil
}

func (r *UserFileRepository) UpdatePublished(objId string, setValPublished bool) (*UserFileDto, error) {
	database := utils.GetMongoDatabase(r.mongo)
	var collection *mongo.Collection = database.Collection(collectionGlobalObjects)

	findDocument, err := GetIdDoc(objId)
	if err != nil {
		return nil, err
	}

	updateDocument := GetUpdateDoc(primitive.M{published: setValPublished})

	one := collection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return nil, errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return nil, one.Err()
	}
	var elem UserFileDto
	if err := one.Decode(&elem); err != nil {
		return nil, err
	}
	return &elem, nil
}

func (r *UserFileRepository) FindUserFiles(userIdInt int) (*mongo.Cursor, error) {
	database := utils.GetMongoDatabase(r.mongo)
	var collection *mongo.Collection = database.Collection(collectionGlobalObjects)
	return collection.Find(context.TODO(), bson.D{{userId, userIdInt}})
}

func (r *UserFileRepository) Delete(objId string) error {
	database := utils.GetMongoDatabase(r.mongo)
	var collection *mongo.Collection = database.Collection(collectionGlobalObjects)
	d, e := GetIdDoc(objId)
	if e != nil {
		return e
	}
	_, e = collection.DeleteOne(context.TODO(), d)
	return e

}

func IsDocumentExists(mongoC *mongo.Client, collection string, request interface{}, opts ...*options.FindOneOptions) (bool, error) {
	database := utils.GetMongoDatabase(mongoC)

	// https://siongui.github.io/2017/03/13/go-pass-slice-or-array-as-variadic-parameter/#id12
	res := database.Collection(collection).FindOne(context.TODO(), request, opts[:]...)
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return false, nil
		}
		Logger.Errorf("Error during find '%v' : %v", request, res.Err())
		return false, res.Err()
	}

	_, e := res.DecodeBytes()

	if e != nil {
		Logger.Errorf("Error during DecodeBytes '%v' : %v", request, res.Err())
		return false, e
	} else {
		return true, nil
	}
}

func (r *LimitsRepository) IsStorageUnlimitedForUser(userId int) (bool, error) {
	return IsDocumentExists(r.mongo, collectionLimits, bson.D{{Id, userId}})
}
