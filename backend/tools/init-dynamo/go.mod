module init-dynamo

go 1.25.0

require (
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.18.2
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.40.1
	github.com/google/uuid v1.6.0
	github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws v0.0.0
)

replace github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws => ../../pkg/aws
