# Go Civitai Downloader

## Running On RunPod

1. Create a Civitai API key: https://civitai.com/user/account
2. Sign up for a RunPod account: https://www.runpod.io/
3. Create a new pod using this template: https://runpod.io/console/deploy?template=1yue39xvu3&ref=ggryk725
4. Click `Edit Template`
5. Expand the `Public Environment Variables` section
6. Enter your API key as `CIVITAI_API_KEY`
7. Enter your username (or a comma-separated list of usernames) as `CIVITAI_USERNAME`
8. Start the pod and watch the logs until it has finished downloading your content
9. Click 

## Overview

This is a command-line tool written in Go to download models from Civitai.com based on specified criteria. It features a two-phase download process (metadata scan + confirmation), concurrent downloads, local database tracking, file verification, and flexible configuration via a file and command-line flags.

## Features

*   **Criteria-Based Downloading:** Fetch models using filters like type, base model, NSFW status, query terms, tags, usernames, etc.
*   **Two-Phase Download:**
    1.  Scans the API based on criteria, checks against the local database, and identifies files *to be* downloaded.
    2.  Presents a summary (file count, total size) and asks for user confirmation before starting downloads.
*   **Concurrent Downloads:** Downloads multiple files simultaneously (configurable concurrency level) for faster fetching.
*   **Local Database:** Uses a Bitcask key/value store (default: `civitai_download_db`) to track successfully downloaded files (keyed by **Model Version ID**, e.g., `v_12345`), preventing redownloads and storing status (`Pending`, `Downloaded`, `Error`).
*   **Gzip Compression:** Database entries are compressed using gzip for reduced storage space.
*   **Database Management Commands:**
    *   `db view`: List entries recorded in the database, including their **status** and **version ID key**.
    *   `db verify`: Check if files recorded in the database exist on disk and optionally verify their hashes. Includes status in log messages.
    *   `db search [QUERY]`: Search database entries by model name, showing **status** and **version ID key**.
    *   `db redownload [VERSION_ID]`: Attempt to redownload a specific file using its **Model Version ID**.
*   **Metadata Saving:** Optionally saves a `.json` file containing model/version/file metadata alongside each downloaded file.
*   **Configuration File:** Uses `config.toml` for persistent settings.
*   **Command-Line Flags:** Allows overriding most configuration settings via CLI flags.
*   **Robust API Interaction:** Handles API rate limiting (429) with exponential backoff and retries, uses cursor pagination for deep results, and logs API interactions optionally to `api.log`.
*   **Error Handling:** Includes specific error types for API and download issues.
*   **Structured Logging:** Uses Logrus for leveled logging (configurable via flags).
*   **Interactive Progress:** Uses uilive to show concurrent download progress.
*   **Torrent Generation:** Command to generate `.torrent` and optional magnet link files for downloaded model directories.
*   **Search Indexing (Experimental):** Uses Bleve to index downloaded items (metadata, file paths, torrent info) for potential future search features.

## Change Log

### 27 August 2025

*   **Directory Structure Refinement:** Further adjustments to paths, these make the most sense now considering there can be different base models for the same model:
    *   Model downloads are now saved to `{SavePath}/{type}/{modelName}/{baseModel}/{versionID}-{fileNameSlug}/{slugifiedOriginalFileName}.{ext}`.
    *   Model info (`--model-info`) is saved to `{SavePath}/{type}/{modelName}/{modelID}-{modelNameSlug}.json`.
    *   Model images (`--model-images`) are saved to `{SavePath}/{type}/{modelName}/images/{versionID}/{imageID}.ext`.
    *   Version images (`--version-images`) path structure relative to the model file remains the same: `{SavePath}/{type}/{modelName}/{baseModel}/{versionID}-{fileNameSlug}/images/{imageID}.ext`.

*   **Directory Structure Changes:** The structure of the downloaded files has changed to better reflect the model type and version. This may affect your existing downloads and running this application.
    *   ~~Model downloads are now saved to `{type}/{baseModel}/{modelName}/{modelID}-{fileNameSlug}/{versionName}/{versionID}_{fileName}.ext`.~~
    * If the model information arguments are passed, this will save in the same directory as the model file, with the versions.
      * Running `--model-images` and `--version-images` might double up with images. Both are available if you just wanted to scrape model metadata and images.
    *   ~~Model info (`--model-info`) is saved to `{type}/{baseModel}/{modelName}/{modelID}-{modelNameSlug}.json`.~~
    *   ~~Model images (`--model-images`) are saved to `{type}/{baseModel}/{modelName}/images/{versionID}/{imageID}.ext`.~~
    *   ~~Version images (`--version-images`) are saved to `{type}/{baseModel}/{modelName}/{versionID}-{fileNameSlug}/images/{imageID}.ext`.~~
    *   We no longer use the `models_info` directory due to the new structure.
*   **Version Metadata JSON:** The JSON file saved with `--metadata` now contains the full, unmodified `ModelVersion` data from the API. Previously this information was not complete.
* The `--limit` will now stop after it's reached, and not continue to cycle over pagination.
* It also seems the civitai API had incorrect file extensions, for example an image could be listed as .jpeg but actually is .webp :(. This has been fixed to use the correct file extension.
* Fixed an issue where config.toml file values were not being properly carried through.
* Fixed downloads not verifying filenames properly when ran twice, which resulted in re downloads.
* Added a `torrent` command to generate `.torrent` files (and optional magnet links) for downloaded model directories, allowing for peer-to-peer sharing. Supports multiple tracker URLs, concurrency, and overwriting.
* Integrated Bleve search indexing. Model metadata, file paths, and torrent information are now indexed when models are downloaded or torrents are generated. This lays the groundwork for future search capabilities (currently no direct user-facing search command via Bleve).

### 26 August 2025

* *Big* refactor for download and image modules.
* **Important:** There have been changes to some of the argument names and config names to simplify them, refer to Configuration section for the new names.
* New `--model-version-id` for `download` and `images` to target a specific version ID. This will generally override some other arguments.
* Similarly, there is now a `--model-id` which will target an entire model.
* When downloading images with `--model-images` and `--version-images` this now uses the concurrency amount set.
* Added a `clean` command which will scan the downloads directory and remove any .tmp files left over from failed or cancelled downloads. This also can remove .torrent and -magnet.txt files.
* The `db verify` command will now return what models are missing or have invalid hashes, and prompt the user to redownload them.
* A new `--all-versions` flag for `download` which will download all versions of a model, not just the latest. The latest is by default.
* The `torrent` command can now generate torrent files concurrently.

## Caveats

Civitai is a beast of its own, especially the API. There are some things to note:

* The api information returned sometimes is inaccurate, hash values can sometimes be incorrect, or required fields for this app to function are missing.
* I've tested this fine downloading all WAN Video LORAs, but I can't guarantee it will work for all model categories. So far so good.
* Sometimes .tmp files are left over, probably due to failed hash or downloads. You can run `clean` to remove them.

## Building

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/dreamfast/go-civitai-downloader.git
    cd go-civitai-downloader
    ```

2.  **Build using Make (Recommended):**
    A `Makefile` is provided for convenience.
    ```bash
    make build
    ```
    This creates the `civitai-downloader` binary in the project root.

3.  **Build using Go:**
    ```bash
    go build -o civitai-downloader ./cmd/civitai-downloader
    ```

*   **Clean build artifacts:**
    ```bash
    make clean
    ```

## Configuration (`config.toml`)

The application uses a `config.toml` file (default location in the same directory as the executable) for settings. You can specify a different path using the `--config` flag.

Generally arguments passed into the application will override the config file settings. An example `config.toml.example` is provided in the repository, simply rename it to `config.toml` and edit the values as needed.

| Option                  | Type       | Default              | Description                                                                                             |
| :---------------------- | :--------- | :------------------- | :------------------------------------------------------------------------------------------------------ |
| `ApiKey`                | `string`   | `""`                 | Your Civitai API Key (Required for downloading models).                                                  |
| `SavePath`              | `string`   | `"downloads"`        | Root directory where model subdirectories (like `lora/sdxl_1.0/mymodel/`) will be saved.                 |
| `DatabasePath`          | `string`   | `""`                 | Path to the database file. If empty, defaults to `[SavePath]/civitai_download_db`.                      |
| `BleveIndexPath`        | `string`   | `""`                 | Path to the Bleve search index directory. If empty, defaults to `[SavePath]/civitai.bleve`.            |
| `Query`                 | `string`   | `""`                 | Default search query string.                                                                            |
| `Tags`                  | `[]string` | `[]`                 | Default list of tags to filter by (Currently only supports single tag via `--tag` flag).              |
| `Usernames`             | `[]string` | `[]`                 | Default list of usernames to filter by (Currently only supports single username via `--username` flag). |
| `ModelTypes`            | `[]string` | `[]`                 | Default model types to query (e.g., `["Checkpoint", "LORA"]`). Empty means all types.                |
| `BaseModels`            | `[]string` | `[]`                 | Default base models to query (e.g., `["SDXL 1.0"]`). Empty means all base models.                     |
| `IgnoreBaseModels`      | `[]string` | `[]`                 | List of base model strings to ignore (case-insensitive substring match). (`--ignore-base-models` flag) |
| `Nsfw`                  | `bool`     | `false`              | Default setting for including NSFW models in API queries.                                               |
| `ModelVersionID`        | `int`      | `0`                  | Default model version ID to download (0 = disabled, overrides other filters).                           |
| `AllVersions`           | `bool`     | `false`              | Download all versions of matched models, not just the latest. (`--all-versions` flag)                   |
| `PrimaryOnly`           | `bool`     | `false`              | Only download the file marked as "primary" for a model version. (`--primary-only` flag)                 |
| `Pruned`                | `bool`     | `false`              | For Checkpoint models, only download files marked as "pruned". (`--pruned` flag)                        |
| `Fp16`                  | `bool`     | `false`              | For Checkpoint models, only download files marked as "fp16". (`--fp16` flag)                           |
| `IgnoreFileNameStrings` | `[]string` | `[]`                 | List of strings to ignore in filenames (case-insensitive substring match). (`--ignore-filename-strings` flag) |
| `Sort`                  | `string`   | `"Most Downloaded"`  | Default sort order for API queries ("Highest Rated", "Most Downloaded", "Newest"). (`--sort` flag)      |
| `Period`                | `string`   | `"AllTime"`          | Default time period for sorting ("AllTime", "Year", "Month", "Week", "Day"). (`--period` flag)        |
| `Limit`                 | `int`      | `100`                | Default models per API page (1-100). (`--limit` flag)                                                   |
| `MaxPages`              | `int`      | `0`                  | Default maximum number of API pages to fetch (0 for no limit). (`--max-pages` flag)                     |
| `Concurrency`           | `int`      | `4`                  | Default number of concurrent downloads. (`--concurrency` flag)                                          |
| `Metadata`              | `bool`     | `false`              | Save a `.json` metadata file (containing the full version details) alongside downloads (overrides config `Metadata`).
| `MetaOnly`              | `bool`     | `false`              | Scan, check DB, and save *only* the `.json` metadata files for potential downloads, skipping the actual model file download and confirmation prompt. Useful with `--model-info`.
| `ModelInfo`             | `bool`     | `false`              | Save full model info JSON to `{SavePath}/{type}/{modelName}/{modelID}-{modelNameSlug}.json`. (`--model-info` flag)                          |
| `VersionImages`         | `bool`     | `false`              | Download images associated with the specific downloaded version into `{SavePath}/{type}/{modelName}/{baseModel}/{versionID}-{fileNameSlug}/images/`. (`--version-images` flag)              |
| `ModelImages`           | `bool`     | `false`              | When `ModelInfo` is true, also download all images for all versions into `{SavePath}/{type}/{modelName}/images/`. (`--model-images` flag)           |
| `SkipConfirmation`      | `bool`     | `false`              | Skip the confirmation prompt before downloading. (`--yes` flag)                                       |
| `ApiDelayMs`            | `int`      | `200`                | Polite delay (milliseconds) between API metadata requests. (`--api-delay` flag)                         |
| `ApiClientTimeoutSec`   | `int`      | `60`                 | Timeout (seconds) for API HTTP client requests. (`--api-timeout` flag)                                  |
| `LogApiRequests`        | `bool`     | `false`              | Log API request/response details to `api.log`. (`--log-api` flag)         |

### Categories and Config Validation

At the moment the categories for BaseModels must be one of the following:

| Base Models       |                   |                    |                   |
| :---------------- | :---------------- | :----------------- | :---------------- |
| `SD 1.4`          | `SD 1.5`          | `SD 1.5 LCM`       | `SD 1.5 Hyper`    |
| `SD 2.0`          | `SD 2.0 768`      | `SD 2.1`           | `SD 2.1 768`      |
| `SD 2.1 Unclip`   | `SDXL 0.9`        | `SDXL 1.0`         | `SD 3`            |
| `SD 3.5`          | `SD 3.5 Medium`   | `SD 3.5 Large`     | `SD 3.5 Large Turbo`|
| `Pony`            | `Flux.1 S`        | `Flux.1 D`         | `AuraFlow`        |
| `SDXL 1.0 LCM`    | `SDXL Distilled`  | `SDXL Turbo`       | `SDXL Lightning`  |
| `SDXL Hyper`      | `Stable Cascade`  | `SVD`              | `SVD XT`          |
| `Playground v2`   | `PixArt a`        | `PixArt E`         | `Hunyuan 1`       |
| `Hunyuan Video`   | `Lumina`          | `Kolors`           | `Illustrious`     |
| `Mochi`           | `LTXV`            | `CogVideoX`        | `NoobAI`          |
| `Wan Video`       | `HiDream`         | `Other`            |                   |

At the moment there isn't very good validation for passing information to the API, so you must ensure that the values, like "LORA" are correct. It is important to note that these values are case sensitive.

If you run any into problems, I suggest to enable the API logging and debug logging to get a better idea of what the problem is.

## Usage

The application is run via the command line.

```bash
./civitai-downloader [global flags] [command] [command flags]
```

**Global Flags:**

*   `--config string`: Path to the configuration file (default \"config.toml\")
*   `--log-level string`: Logging level (debug, info, warn, error) (default \"info\")
*   `--log-format string`: Logging format (text, json) (default \"text\")
*   `--log-api`: Log API requests/responses to `api.log` (overrides config `LogApiRequests`)
*   `--save-path string`: Override the `SavePath` from the config file.
*   `--api-timeout int`: Override `ApiClientTimeoutSec` from config (seconds).
*   `--api-delay int`: Override `ApiDelayMs` from config (milliseconds).
*   `--db-path string`: Override `DatabasePath` from config.
*   `--index-path string`: Override `BleveIndexPath` from config.

**Commands:**

### `download`

Scans the Civitai API based on filters, asks for confirmation, and then downloads new models.

```bash
./civitai-downloader download [flags]
```

**`download` Flags (override `config.toml`):**

*   `-t, --type strings`: Filter by model type(s) (e.g., Checkpoint, LORA).
*   `-b, --base-model strings`: Filter by base model(s) (e.g., "SD 1.5", SDXL).
*   `--nsfw`: Include NSFW models in query (overrides config `Nsfw`).
*   `-l, --limit int`: Max models per API page (default 100).
*   `-s, --sort string`: Sort order (default "Most Downloaded").
*   `-p, --period string`: Time period for sorting (default "AllTime").
*   `--primary-only`: Only download primary files (overrides config `PrimaryOnly`).
*   `-q, --query string`: Add a search query string.
*   `--tag string`: Filter by specific tag name.
*   `-u, --username string`: Filter by specific username.
*   `--tags strings`: Filter by tags (comma-separated). *(No shorthand)*
*   `--usernames strings`: Filter by usernames (comma-separated). *(No shorthand)*
*   `-m, --model-types strings`: Filter by model types (e.g., Checkpoint, LORA, LoCon).
*   `--model-id int`: Download versions for a specific model ID (overrides general filters like query, tags). *(No shorthand)*
*   `--model-version-id int`: Download a specific model version ID (overrides model-id and general filters). *(No shorthand)*
*   `--pruned`: Only download pruned Checkpoints (overrides config `Pruned`).
*   `--fp16`: Only download fp16 Checkpoints (overrides config `Fp16`).
*   `--ignore-base-models strings`: Base models to ignore (comma-separated or multiple flags, overrides config `IgnoreBaseModels`). *(No shorthand)*
*   `--ignore-filename-strings strings`: Substrings in filenames to ignore (comma-separated or multiple flags, overrides config `IgnoreFileNameStrings`). *(No shorthand)*
*   `-c, --concurrency int`: Number of concurrent downloads (overrides config `Concurrency`).
*   `--max-pages int`: Maximum number of API pages to fetch (0 for no limit). *(No shorthand)*
*   `--metadata`: Save a `.json` metadata file (containing the full version details) alongside downloads (overrides config `Metadata`).
*   `-y, --yes`: Skip confirmation prompt before downloading (overrides config `SkipConfirmation`).
*   `--meta-only`: Scan, check DB, and save *only* the `.json` metadata files for potential downloads, skipping the actual model file download and confirmation prompt. Useful with `--model-info`.
*   `--model-info`: During the scan phase, save the *full* JSON data for each model returned by the API to `{SavePath}/{type}/{modelName}/{modelID}-{modelNameSlug}.json`. Overwrites existing files.
*   `--version-images`: After a model file download succeeds, download the associated preview/example images for that specific version into a `{SavePath}/{type}/{modelName}/{baseModel}/{versionID}-{fileNameSlug}/images/` subdirectory.
*   `--model-images`: **Requires `--model-info`.** When saving the full model info JSON, also attempt to download *all* images associated with *all* versions listed in the model info. Images are saved into `{SavePath}/{type}/{modelName}/images/{versionId}/{imageId}.{ext}`.
*   `--all-versions`: Download all versions of a model, not just the latest (overrides version selection and config `AllVersions`).

**Examples:**

*   Download the latest Checkpoint models for SDXL 1.0, increase concurrency, and skip confirmation:
    ```bash
    ./civitai-downloader download --type Checkpoint --base-model "SDXL 1.0" --sort Newest -c 8 -y
    ```

*   Download the latest version of model ID 12345:
    ```bash
    ./civitai-downloader download --model-id 12345
    ```

*   Download all versions of model ID 12345:
    ```bash
    ./civitai-downloader download --model-id 12345 --all-versions
    ```

*   Download all LORA models based on the "Wan Video" base model, saving metadata:
    ```bash
    ./civitai-downloader download --type LORA --base-model "Wan Video" --metadata
    ```

*   Search for models containing "style" in their name, limit to the first 2 pages of results, and filter for SD 1.5 base models:
    ```bash
    ./civitai-downloader download -q style --limit 100 --max-pages 2 --base-model "SD 1.5"
    ```

### `images`

Downloads images directly from the `/api/v1/images` endpoint based on various filters. Does not use the database.

```bash
./civitai-downloader images [flags]
```

**`images` Flags:**

*   `--limit int`: Max images per API page (1-200, default 100).
*   `--post-id int`: Filter by Post ID.
*   `--model-id int`: Filter by Model ID.
*   `--model-version-id int`: Filter by Model Version ID.
*   `-u, --username string`: Filter by username.
*   `--nsfw string`: Filter by NSFW level (None, Soft, Mature, X) or boolean (true/false). Empty means all.
*   `-s, --sort string`: Sort order (Most Reactions, Most Comments, Newest, default "Newest").
*   `-p, --period string`: Time period for sorting (AllTime, Year, Month, Week, Day, default "AllTime").
*   `--max-pages int`: Maximum number of API pages to fetch (0 for no limit).
*   `-o, --output-dir string`: Directory to save images (default `[SavePath]/images/{author}/{baseModel}/`).
*   `-c, --concurrency int`: Number of concurrent image downloads (default 4).
*   `--metadata`: Save a `.json` metadata file (containing the ImageApiItem data) alongside each downloaded image.

**Examples:**

*   Download the most recent 50 images posted by user "exampleUser", saving metadata:
    ```bash
    ./civitai-downloader images -u exampleUser --limit 50 --metadata
    ```

*   Download all images associated with model version ID 12345, saving them to a specific directory:
    ```bash
    ./civitai-downloader images --model-version-id 12345 -o ./downloaded_images
    ```

*   Download the top-rated images for model ID 9876 for the past week:
    ```bash
    ./civitai-downloader images --model-id 9876 -s "Most Reactions" -p Week
    ```

### `db`

Parent command for database operations.

#### `db view`

Lists all model file entries recorded in the database, including their **status** and **version ID key**.

```bash
./civitai-downloader db view
```

#### `db verify`

Checks recorded database entries against the filesystem, providing status context.

```bash
./civitai-downloader db verify [--check-hash=true|false]
```

*   `--check-hash`: Perform hash check for existing files (default true).
*   Also checks/creates `.json` metadata files (if main file exists) if `Metadata` is enabled globally (via config or flag).

#### `db redownload`

Attempts to redownload a specific file using its **Model Version ID**.

```bash
./civitai-downloader db redownload <MODEL_VERSION_ID>
```

#### `db search`

Searches database entries for models whose names contain the provided query text, showing **status** and **version ID key**. *(Assumes command exists/is updated)*

```bash
./civitai-downloader db search <MODEL_NAME_QUERY>
```

### `clean`

Scans the configured download directory (`SavePath`) recursively and removes any temporary files ending with `.tmp`.

```bash
./civitai-downloader clean [flags]
```

**`clean` Flags:**

*   `-t, --torrents`: Also remove any `*.torrent` files found during the scan.
*   `-m, --magnets`: Also remove any `*-magnet.txt` files found during the scan.

This command is useful for cleaning up leftover temporary files that might occur due to interrupted downloads or other issues, as well as optionally clearing out generated torrent/magnet files.

### `torrent`

Generates BitTorrent `.torrent` files for models previously downloaded and recorded in the database. This requires access to the downloaded files and the database.

```bash
./civitai-downloader torrent --announce <tracker_url> [flags]
```

**`torrent` Flags:**

*   `--announce strings`: **Required.** Tracker announce URL(s). Can be repeated for multiple trackers.
*   `--model-id ints`: Generate torrents only for specific model ID(s). Can be repeated or comma-separated (e.g., `--model-id 123 --model-id 456` or `--model-id 123,456`). Default: all downloaded models in the database.
*   `-o, --output-dir string`: Directory to save generated .torrent files (default: place inside each model's directory).
*   `-f, --overwrite`: Overwrite existing .torrent files.
*   `-c, --concurrency int`: Number of concurrent torrent generation workers (default 4, binds to global `--concurrency` if not set).
*   `--magnet-links`: Generate a .txt file containing the magnet link alongside each .torrent file (default false).

**Examples:**

*   Generate torrents for all downloaded models, announcing to two trackers, saving torrents into the model directories:
    ```bash
    ./civitai-downloader torrent --announce udp://tracker.opentrackr.org:1337/announce --announce udp://tracker.openbittorrent.com:6969/announce
    ```

*   Generate a torrent only for model ID 12345, overwriting any existing .torrent file, placing it in a specific output directory:
    ```bash
    ./civitai-downloader torrent --announce udp://tracker.opentrackr.org:1337/announce --model-id 12345 -f -o ./torrents
    ```

*   Generate torrents for all models and create corresponding magnet link files next to them:
    ```bash
    ./civitai-downloader torrent --announce udp://tracker.opentrackr.org:1337/announce --magnet-links
    ```

### Torrent Trackers

BitTorrent trackers are servers that help peers find each other to share a torrent's content. While private trackers exist, there are also public trackers available. A good, frequently updated list of public trackers can be found at the [ngosang/trackerslist](https://github.com/ngosang/trackerslist) repository.

You can specify multiple trackers using the `--announce` flag repeatedly. This increases the chances of peers finding each other.

**Example using multiple public trackers:**

```bash
./civitai-downloader torrent \
  -c 12 \
  -f \
  --magnet-links \
  --announce udp://tracker.opentrackr.org:1337/announce \
  --announce udp://open.demonii.com:1337/announce \
  --announce udp://open.stealth.si:80/announce \
  --announce udp://tracker.torrent.eu.org:451/announce \
  --announce udp://explodie.org:6969/announce \
  --announce udp://exodus.desync.com:6969/announce \
  --announce udp://open.free-tracker.ga:6969/announce \
  --announce udp://leet-tracker.moe:1337/announce \
  --announce udp://isk.richardsw.club:6969/announce \
  --announce udp://discord.heihachi.pw:6969/announce \
  --announce http://www.torrentsnipe.info:2701/announce \
  --announce http://www.genesis-sp.org:2710/announce \
  --announce http://tracker810.xyz:11450/announce \
  --announce http://tracker.xiaoduola.xyz:6969/announce \
  --announce http://tracker.vanitycore.co:6969/announce \
  --announce http://tracker.skyts.net:6969/announce \
  --announce http://tracker.sbsub.com:2710/announce \
  --announce http://tracker.moxing.party:6969/announce \
  --announce http://tracker.lintk.me:2710/announce \
  --announce http://tracker.ipv6tracker.org:80/announce
```

### `search`

Searches the local Bleve index for downloaded items (models, images, etc.) based on a query string.

```bash
./civitai-downloader search [flags] <QUERY>
```

**`search` Flags:**

*   The query `-q|--query` uses [Bleve query string syntax](https://blevesearch.com/docs/Query-String-Query/). You can search specific fields using `+field:value`.

**Indexed Fields (Examples):** `id`, `type`, `name`, `modelName`, `versionName`, `baseModel`, `creatorName`, `tags`, `prompt`, `nsfwLevel`, `fileFormat`, `filePrecision`, `fileSizeType`, `torrentPath`, `magnetLink`.

**Examples:**

*   Search for items with "lora" in any indexed field:
    ```bash
    ./civitai-downloader search lora
    ```

*   Search specifically for models named "Dreamwood":
    ```bash
    ./civitai-downloader search +modelName:Dreamwood
    ```

*   Search for items tagged with "style":
    ```bash
    ./civitai-downloader search +tags:style
    ```

*   Search for Checkpoint models:
    ```bash
    ./civitai-downloader search +type:Checkpoint
    ```

*   Search for Safetensor files:
    ```bash
    ./civitai-downloader search +fileFormat:safetensor
    ```

## Project Structure

*   `cmd/civitai-downloader/`: Main application entry point and Cobra command definitions.
*   `internal/`: Internal packages not intended for external use.
    *   `api/`: Civitai API client logic.
    *   `config/`: Configuration loading.
    *   `database/`: Bitcask database interaction wrapper (including Gzip).
    *   `downloader/`: File downloading logic (handles auth, temp files, hash check).
    *   `helpers/`: Utility functions.
    *   `models/`: Struct definitions for config, API responses, database entries.
*   `index/`: Bleve search indexing logic and item definition.
*   `Makefile`: Build/run/test/clean automation.
*   `config.toml`: Default configuration file.


## Dependencies

*   [github.com/spf13/cobra](https://github.com/spf13/cobra): CLI framework.
*   [github.com/spf13/viper](https://github.com/spf13/viper): Configuration management (used for flag binding).
*   [github.com/BurntSushi/toml](https://github.com/BurntSushi/toml): TOML configuration parsing.
*   [github.com/sirupsen/logrus](https://github.com/sirupsen/logrus): Structured logging.
*   [github.com/gosuri/uilive](https://github.com/gosuri/uilive): Terminal live writer for progress.
*   [git.mills.io/prologic/bitcask](https://git.mills.io/prologic/bitcask): Embedded key/value database.
*   [lukechampine.com/blake3](https://lukechampine.com/blake3): BLAKE3 hashing.
*   [github.com/anacrolix/torrent](https://github.com/anacrolix/torrent): BitTorrent library (metainfo, bencode).
*   [github.com/blevesearch/bleve](https://github.com/blevesearch/bleve): Full-text search and indexing library. 
