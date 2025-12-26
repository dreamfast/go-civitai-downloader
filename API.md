Introduction

This article describes how to use the Civitai REST API. We are going to be describing the HTTP method, path, and parameters for every operation. The API will return the response status code, response headers, and a response body.

    This is still in active development and will be updated once more endpoints are made available for the public

Civitai API v1

    Authorization

Creators

    GET /api/v1/creators

Images

    GET /api/v1/images

Models

    GET /api/v1/models

Model

    GET /api/v1/models/:modelId

Model Version

    GET /api/v1/model-versions/:modelVersionId
    GET /api/v1/model-versions/by-hash/:hash

Tags

    GET /api/v1/tags

Authorization

To make authorized requests as a user you must use an API Key. You can generate an API Key from your User Account Settings.

Once you have an API Key you can authenticate with either an Authorization Header or Query String.

Creators can require that people be logged in to download their resources. That is an option we provide but not something we require – it's entirely up to the resource owner.

Please see the Guide to Downloading via API for more details and open an issue if you are still having trouble downloading.
Authorization Header

You can pass the API token as a Bearer token using the Authorization header:

GET https://civitai.com/api/v1/models
Content-Type: application/json
Authorization: Bearer {api_key}

Query String

You can pass the API token as a query parameter using the ?token= parameter:

GET https://civitai.com/api/v1/models?token={api_key}
Content-Type: application/json

This method may be easier in some notebooks and scripts.
GET /api/v1/creators
Endpoint URL

https://civitai.com/api/v1/creators
Query Parameters
Name 	Type 	Description
limit (OPTIONAL) 	number 	The number of results to be returned per page. This can be a number between 0 and 200. By default, each page will return 20 results. If set to 0, it'll return all the creators
page (OPTIONAL) 	number 	The page from which to start fetching creators
query (OPTIONAL) 	string 	Search query to filter creators by username
Response Fields
Name 	Type 	Description
username 	string 	The username of the creator
modelCount 	number 	The amount of models linked to this user
link 	string 	Url to get all models from this user
metadata.totalItems 	string 	The total number of items available
metadata.currentPage 	string 	The the current page you are at
metadata.pageSize 	string 	The the size of the batch
metadata.totalPages 	string 	The total number of pages
metadata.nextPage 	string 	The url to get the next batch of items
metadata.prevPage 	string 	The url to get the previous batch of items
Example

The following example shows a request to get the first 3 model tags from our database:

curl https://civitai.com/api/v1/creators?limit=3 \
-H "Content-Type: application/json" \
-X GET

This would yield the following response:

{
  "items": [
    {
      "username": "Civitai",
      "modelCount": 848,
      "link": "https://civitai.com/api/v1/models?username=Civitai"
    },
    {
      "username": "JustMaier",
      "modelCount": 8,
      "link": "https://civitai.com/api/v1/models?username=JustMaier"
    },
    {
      "username": "maxhulker",
      "modelCount": 2,
      "link": "https://civitai.com/api/v1/models?username=maxhulker"
    }
  ],
  "metadata": {
    "totalItems": 46,
    "currentPage": 1,
    "pageSize": 3,
    "totalPages": 16,
    "nextPage": "https://civitai.com/api/v1/creators?limit=3&page=2"
  }
}

GET /api/v1/images
Endpoint URL

https://civitai.com/api/v1/images
Query Parameters
Name 	Type 	Description
limit (OPTIONAL) 	number 	The number of results to be returned per page. This can be a number between 0 and 200. By default, each page will return 100 results.
postId (OPTIONAL) 	number 	The ID of a post to get images from
modelId (OPTIONAL) 	number 	The ID of a model to get images from (model gallery)
modelVersionId (OPTIONAL) 	number 	The ID of a model version to get images from (model gallery filtered to version)
username (OPTIONAL) 	string 	Filter to images from a specific user
nsfw (OPTIONAL) 	boolean | enum (None, Soft, Mature, X) 	Filter to images that contain mature content flags or not (undefined returns all)
sort (OPTIONAL) 	enum (Most Reactions, Most Comments, Newest) 	The order in which you wish to sort the results
period (OPTIONAL) 	enum (AllTime, Year, Month, Week, Day) 	The time frame in which the images will be sorted
page (OPTIONAL) 	number 	The page from which to start fetching creators
Response Fields
Name 	Type 	Description
id 	number 	The id of the image
url 	string 	The url of the image at it's source resolution
hash 	string 	The blurhash of the image
width 	number 	The width of the image
height 	number 	The height of the image
nsfw 	boolean 	If the image has any mature content labels
nsfwLevel 	enum (None, Soft, Mature, X) 	The NSFW level of the image
createdAt 	date 	The date the image was posted
postId 	number 	The ID of the post the image belongs to
stats.cryCount 	number 	The number of cry reactions
stats.laughCount 	number 	The number of laugh reactions
stats.likeCount 	number 	The number of like reactions
stats.heartCount 	number 	The number of heart reactions
stats.commentCount 	number 	The number of comment reactions
meta 	object 	The generation parameters parsed or input for the image
username 	string 	The username of the creator
metadata.nextCursor 	number 	The id of the first image in the next batch
metadata.currentPage 	number 	The the current page you are at (if paging)
metadata.pageSize 	number 	The the size of the batch (if paging)
metadata.nextPage 	string 	The url to get the next batch of items
Example

The following example shows a request to get the first image:

curl https://civitai.com/api/v1/images?limit=1 \
-H "Content-Type: application/json" \
-X GET

This would yield the following response:
Click to Expand

Notes:

    On July 2, 2023 we switch from a paging system to a cursor based system due to the volume of data and requests for this endpoint.
    Whether you use paging or cursors, you can use metadata.nextPage to get the next page of results

GET /api/v1/models
Endpoint URL

https://civitai.com/api/v1/models
Query Parameters
Name 	Type 	Description
limit (OPTIONAL) 	number 	The number of results to be returned per page. This can be a number between 1 and 100. By default, each page will return 100 results
page (OPTIONAL) 	number 	The page from which to start fetching models
query (OPTIONAL) 	string 	Search query to filter models by name
tag (OPTIONAL) 	string 	Search query to filter models by tag
username (OPTIONAL) 	string 	Search query to filter models by user
types (OPTIONAL) 	enum[] (Checkpoint, TextualInversion, Hypernetwork, AestheticGradient, LORA, Controlnet, Poses) 	The type of model you want to filter with. If none is specified, it will return all types
sort (OPTIONAL) 	enum (Highest Rated, Most Downloaded, Newest) 	The order in which you wish to sort the results
period (OPTIONAL) 	enum (AllTime, Year, Month, Week, Day) 	The time frame in which the models will be sorted
rating (OPTIONAL) (Deprecated) 	number 	The rating you wish to filter the models with. If none is specified, it will return models with any rating
favorites (OPTIONAL) (AUTHED) 	boolean 	Filter to favorites of the authenticated user (this requires an API token or session cookie)
hidden (OPTIONAL) (AUTHED) 	boolean 	Filter to hidden models of the authenticated user (this requires an API token or session cookie)
primaryFileOnly (OPTIONAL) 	boolean 	Only include the primary file for each model (This will use your preferred format options if you use an API token or session cookie)
allowNoCredit (OPTIONAL) 	boolean 	Filter to models that require or don't require crediting the creator
allowDerivatives (OPTIONAL) 	boolean 	Filter to models that allow or don't allow creating derivatives
allowDifferentLicenses (OPTIONAL) 	boolean 	Filter to models that allow or don't allow derivatives to have a different license
allowCommercialUse (OPTIONAL) 	enum (None, Image, Rent, Sell) 	Filter to models based on their commercial permissions
nsfw (OPTIONAL) 	boolean 	If false, will return safer images and hide models that don't have safe images
supportsGeneration (OPTIONAL) 	boolean 	If true, will return models that support generation
Response Fields
Name 	Type 	Description
id 	number 	The identifier for the model
name 	string 	The name of the model
description 	string 	The description of the model (HTML)
type 	enum (Checkpoint, TextualInversion, Hypernetwork, AestheticGradient, LORA, Controlnet, Poses) 	The model type
nsfw 	boolean 	Whether the model is NSFW or not
tags 	string[] 	The tags associated with the model
mode 	enum (Archived, TakenDown) | null 	The mode in which the model is currently on. If Archived, files field will be empty. If TakenDown, images field will be empty
creator.username 	string 	The name of the creator
creator.image 	string | null 	The url of the creators avatar
stats.downloadCount 	number 	The number of downloads the model has
stats.favoriteCount 	number 	The number of favorites the model has
stats.commentCount 	number 	The number of comments the model has
stats.ratingCount 	number 	The number of ratings the model has
stats.rating 	number 	The average rating of the model
modelVersions.id 	number 	The identifier for the model version
modelVersions.name 	string 	The name of the model version
modelVersions.description 	string 	The description of the model version (usually a changelog)
modelVersions.createdAt 	Date 	The date in which the version was created
modelVersions.downloadUrl 	string 	The download url to get the model file for this specific version
modelVersions.trainedWords 	string[] 	The words used to trigger the model
modelVersions.files.sizeKb 	number 	The size of the model file
modelVersions.files.pickleScanResult 	string 	Status of the pickle scan ('Pending', 'Success', 'Danger', 'Error')
modelVersions.files.virusScanResult 	string 	Status of the virus scan ('Pending', 'Success', 'Danger', 'Error')
modelVersions.files.scannedAt 	Date | null 	The date in which the file was scanned
modelVersions.files.primary 	boolean | undefined 	If the file is the primary file for the model version
modelVersions.files.metadata.fp 	enum (fp16, fp32) | undefined 	The specified floating point for the file
modelVersions.files.metadata.size 	enum (full, pruned) | undefined 	The specified model size for the file
modelVersions.files.metadata.format 	enum (SafeTensor, PickleTensor, Other) | undefined 	The specified model format for the file
modelVersions.images.id 	string 	The id for the image
modelVersions.images.url 	string 	The url for the image
modelVersions.images.nsfw 	string 	Whether or not the image is NSFW (note: if the model is NSFW, treat all images on the model as NSFW)
modelVersions.images.width 	number 	The original width of the image
modelVersions.images.height 	number 	The original height of the image
modelVersions.images.hash 	string 	The blurhash of the image
modelVersions.images.meta 	object | null 	The generation params of the image
modelVersions.stats.downloadCount 	number 	The number of downloads the model has
modelVersions.stats.ratingCount 	number 	The number of ratings the model has
modelVersions.stats.rating 	number 	The average rating of the model
metadata.totalItems 	string 	The total number of items available
metadata.currentPage 	string 	The the current page you are at
metadata.pageSize 	string 	The the size of the batch
metadata.totalPages 	string 	The total number of pages
metadata.nextPage 	string 	The url to get the next batch of items
metadata.prevPage 	string 	The url to get the previous batch of items

Note: The download url uses a content-disposition header to set the filename correctly. Be sure to enable that header when fetching the download. For example, with wget:

wget https://civitai.com/api/download/models/{modelVersionId} --content-disposition

If the creator of the asset that you are trying to download requires authentication, then you will need an API Key to download it:

wget https://civitai.com/api/download/models/{modelVersionId}?token={api_key} --content-disposition

Example

The following example shows a request to get the first 3 TextualInversion models from our database:

curl https://civitai.com/api/v1/models?limit=3&types=TextualInversion \
-H "Content-Type: application/json" \
-X GET

This would yield the following response:
Click to expand

GET /api/v1/models/:modelId
Endpoint URL

https://civitai.com/api/v1/models/:modelId
Response Fields
Name 	Type 	Description
id 	number 	The identifier for the model
name 	string 	The name of the model
description 	string 	The description of the model (HTML)
type 	enum (Checkpoint, TextualInversion, Hypernetwork, AestheticGradient, LORA, Controlnet, Poses) 	The model type
nsfw 	boolean 	Whether the model is NSFW or not
tags 	string[] 	The tags associated with the model
mode 	enum (Archived, TakenDown) | null 	The mode in which the model is currently on. If Archived, files field will be empty. If TakenDown, images field will be empty
creator.username 	string 	The name of the creator
creator.image 	string | null 	The url of the creators avatar
modelVersions.id 	number 	The identifier for the model version
modelVersions.name 	string 	The name of the model version
modelVersions.description 	string 	The description of the model version (usually a changelog)
modelVersions.createdAt 	Date 	The date in which the version was created
modelVersions.downloadUrl 	string 	The download url to get the model file for this specific version
modelVersions.trainedWords 	string[] 	The words used to trigger the model
modelVersions.files.sizeKb 	number 	The size of the model file
modelVersions.files.pickleScanResult 	string 	Status of the pickle scan ('Pending', 'Success', 'Danger', 'Error')
modelVersions.files.virusScanResult 	string 	Status of the virus scan ('Pending', 'Success', 'Danger', 'Error')
modelVersions.files.scannedAt 	Date | null 	The date in which the file was scanned
modelVersions.files.metadata.fp 	enum (fp16, fp32) | undefined 	The specified floating point for the file
modelVersions.files.metadata.size 	enum (full, pruned) | undefined 	The specified model size for the file
modelVersions.files.metadata.format 	enum (SafeTensor, PickleTensor, Other) | undefined 	The specified model format for the file
modelVersions.images.url 	string 	The url for the image
modelVersions.images.nsfw 	string 	Whether or not the image is NSFW (note: if the model is NSFW, treat all images on the model as NSFW)
modelVersions.images.width 	number 	The original width of the image
modelVersions.images.height 	number 	The original height of the image
modelVersions.images.hash 	string 	The blurhash of the image
modelVersions.images.meta 	object | null 	The generation params of the image

Note: The download url uses a content-disposition header to set the filename correctly. Be sure to enable that header when fetching the download. For example, with wget:

wget https://civitai.com/api/download/models/{modelVersionId} --content-disposition

Example

The following example shows a request to get the first 3 TextualInversion models from our database:

curl https://civitai.com/api/v1/models/1102 \
-H "Content-Type: application/json" \
-X GET

This would yield the following response:
Click to expand

GET /api/v1/models-versions/:modelVersionId
Endpoint URL

https://civitai.com/api/v1/model-versions/:id
Response Fields
Name 	Type 	Description
id 	number 	The identifier for the model version
name 	string 	The name of the model version
description 	string 	The description of the model version (usually a changelog)
model.name 	string 	The name of the model
model.type 	enum (Checkpoint, TextualInversion, Hypernetwork, AestheticGradient, LORA, Controlnet, Poses) 	The model type
model.nsfw 	boolean 	Whether the model is NSFW or not
model.poi 	boolean 	Whether the model is of a person of interest or not
model.mode 	enum (Archived, TakenDown) | null 	The mode in which the model is currently on. If Archived, files field will be empty. If TakenDown, images field will be empty
modelId 	number 	The identifier for the model
createdAt 	Date 	The date in which the version was created
downloadUrl 	string 	The download url to get the model file for this specific version
trainedWords 	string[] 	The words used to trigger the model
files.sizeKb 	number 	The size of the model file
files.pickleScanResult 	string 	Status of the pickle scan ('Pending', 'Success', 'Danger', 'Error')
files.virusScanResult 	string 	Status of the virus scan ('Pending', 'Success', 'Danger', 'Error')
files.scannedAt 	Date | null 	The date in which the file was scanned
files.metadata.fp 	enum (fp16, fp32) | undefined 	The specified floating point for the file
files.metadata.size 	enum (full, pruned) | undefined 	The specified model size for the file
files.metadata.format 	enum (SafeTensor, PickleTensor, Other) | undefined 	The specified model format for the file
stats.downloadCount 	number 	The number of downloads the model has
stats.ratingCount 	number 	The number of ratings the model has
stats.rating 	number 	The average rating of the model
images.url 	string 	The url for the image
images.nsfw 	string 	Whether or not the image is NSFW (note: if the model is NSFW, treat all images on the model as NSFW)
images.width 	number 	The original width of the image
images.height 	number 	The original height of the image
images.hash 	string 	The blurhash of the image
images.meta 	object | null 	The generation params of the image

Note: The download url uses a content-disposition header to set the filename correctly. Be sure to enable that header when fetching the download. For example, with wget:

wget https://civitai.com/api/download/models/{modelVersionId} --content-disposition

Example

The following example shows a request to get a model version from our database:

curl https://civitai.com/api/v1/model-versions/1318 \
-H "Content-Type: application/json" \
-X GET

This would yield the following response:
Click to expand

GET /api/v1/models-versions/by-hash/:hash
Endpoint URL

https://civitai.com/api/v1/model-versions/by-hash/:hash
Response Fields

Same as standard model-versions endpoint

Note: We support the following hash algorithms: AutoV1, AutoV2, SHA256, CRC32, and Blake3

Note 2: We are still in the process of hashing older files, so these results are incomplete
GET /api/v1/tags
Endpoint URL

https://civitai.com/api/v1/tags
Query Parameters
Name 	Type 	Description
limit (OPTIONAL) 	number 	The number of results to be returned per page. This can be a number between 1 and 200. By default, each page will return 20 results. If set to 0, it'll return all the tags
page (OPTIONAL) 	number 	The page from which to start fetching tags
query (OPTIONAL) 	string 	Search query to filter tags by name
Response Fields
Name 	Type 	Description
name 	string 	The name of the tag
modelCount 	number 	The amount of models linked to this tag
link 	string 	Url to get all models from this tag
metadata.totalItems 	string 	The total number of items available
metadata.currentPage 	string 	The the current page you are at
metadata.pageSize 	string 	The the size of the batch
metadata.totalPages 	string 	The total number of pages
metadata.nextPage 	string 	The url to get the next batch of items
metadata.prevPage 	string 	The url to get the previous batch of items
Example

The following example shows a request to get the first 3 model tags from our database:

curl https://civitai.com/api/v1/tags?limit=3 \
-H "Content-Type: application/json" \
-X GET

This would yield the following response:

{
  "items": [
    {
      "name": "Pepe Larraz",
      "modelCount": 1,
      "link": "https://civitai.com/api/v1/models?tag=Pepe Larraz"
    },
    {
      "name": "comic book",
      "modelCount": 7,
      "link": "https://civitai.com/api/v1/models?tag=comic book"
    },
    {
      "name": "style",
      "modelCount": 91,
      "link": "https://civitai.com/api/v1/models?tag=style"
    }
  ],
  "metadata": {
    "totalItems": 200,
    "currentPage": 1,
    "pageSize": 3,
    "totalPages": 67,
    "nextPage": "https://civitai.com/api/v1/tags?limit=3&page=2"
  }
}

---

## Implementation Notes

This section documents important implementation details and discoveries for developers building API clients.

### Field Type Variations

#### `allowCommercialUse` Field

The `allowCommercialUse` field in model responses can be returned as either a **single string** or an **array of strings**, depending on the model. Implementations should handle both formats:

```go
// Example: Handle both string and []string
type StringOrStringSlice []string

func (s *StringOrStringSlice) UnmarshalJSON(data []byte) error {
    var str string
    if err := json.Unmarshal(data, &str); err == nil {
        *s = []string{str}
        return nil
    }
    var arr []string
    if err := json.Unmarshal(data, &arr); err != nil {
        return err
    }
    *s = arr
    return nil
}
```

### HTTP Request Requirements

#### User-Agent Header

Requests to the Civitai API (especially download URLs) may return **401 Unauthorized** if no User-Agent header is provided. Use a browser-like User-Agent:

```
User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36
```

### Download Authentication

#### Token Query Parameter (Recommended)

For downloading files, use the `?token=` query parameter rather than the `Authorization` header. This is because:

1. Civitai download URLs redirect to AWS S3 for file hosting
2. Cross-domain redirects strip the `Authorization` header for security
3. The query parameter persists through all redirects

```bash
# Correct - token persists through redirects
wget "https://civitai.com/api/download/models/12345?token=YOUR_API_KEY" --content-disposition

# May fail - Authorization header stripped on S3 redirect
curl -H "Authorization: Bearer YOUR_API_KEY" https://civitai.com/api/download/models/12345
```

#### Session Cookie Authentication

Some models require users to be logged in (creator-restricted downloads). For these, you need a browser session cookie:

1. Log into Civitai in your browser
2. Open Developer Tools (F12) → Application → Cookies
3. Copy the value of `__Secure-civitai-token`
4. Pass it in the `Cookie` header:

```
Cookie: __Secure-civitai-token=YOUR_SESSION_COOKIE_VALUE
```

**Important**: Session cookies must be preserved through redirects. Configure your HTTP client to forward cookies on redirect.

### Error Response Detection

When a download fails due to authentication or access restrictions, Civitai returns an HTML error page instead of the file. Implementations should check the `Content-Type` header:

```
Content-Type: text/html  → Error page (not the actual file)
Content-Type: application/octet-stream  → Actual file download
```

Common error scenarios detected via HTML response body:
- **Early Access**: Model requires Supporter membership
- **Login Required**: Creator has restricted downloads to logged-in users
- **Not Found**: Model or version doesn't exist
- **Age Verification**: NSFW content requires verified account