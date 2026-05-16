package cmd

// "github.com/spf13/viper"

// Variables for Image Flags (package level)
var (
	imagesLimitFlag            int
	imagesPostIDFlag           int
	imagesModelIDFlag          int
	imagesModelVersionIDFlag   int
	imagesImageIDFlag          int
	imagesUsernameFlag         string
	imagesNsfwFlag             string
	imagesSortFlag             string
	imagesPeriodFlag           string
	imagesPageFlag             int
	imagesMaxPagesFlag         int
	imagesOutputDirFlag        string
	imagesConcurrencyFlag      int
	imagesMetadataFlag         bool
	imagesDisableImageMimeFlag bool
	imagesBrowsingLevelFlag    int
)

func init() {
	// imagesCmd is defined in images.go
	rootCmd.AddCommand(imagesCmd)

	// --- Flags for Image Command ---
	imagesCmd.Flags().IntVar(&imagesLimitFlag, "limit", 100, "Max images per page (1-200).")
	imagesCmd.Flags().IntVar(&imagesPostIDFlag, "post-id", 0, "Filter by Post ID.")
	imagesCmd.Flags().IntVar(&imagesModelIDFlag, "model-id", 0, "Filter by Model ID.")
	imagesCmd.Flags().IntVar(&imagesModelVersionIDFlag, "model-version-id", 0, "Filter by Model Version ID (overrides model-id and post-id if set).")
	imagesCmd.Flags().IntVar(&imagesImageIDFlag, "image-id", 0, "Filter by specific Image ID.")
	imagesCmd.Flags().StringVarP(&imagesUsernameFlag, "username", "u", "", "Filter by username.")
	// Use string for nsfw flag to handle both boolean and enum values easily
	imagesCmd.Flags().StringVar(&imagesNsfwFlag, flagNsfw, "", "Filter by NSFW level (None, Soft, Mature, X) or boolean (true/false). Empty means all.")
	imagesCmd.Flags().StringVarP(&imagesSortFlag, "sort", "s", "Newest", "Sort order (Most Reactions, Most Comments, Newest).")
	imagesCmd.Flags().StringVarP(&imagesPeriodFlag, "period", "p", "AllTime", "Time period for sorting (AllTime, Year, Month, Week, Day).")
	imagesCmd.Flags().IntVar(&imagesPageFlag, "page", 1, "Starting page number (uses cursor-advance for images API).") // Images API uses cursor-based pagination; Page config triggers cursor-advance
	imagesCmd.Flags().IntVar(&imagesMaxPagesFlag, "max-pages", 0, "Maximum number of API pages to fetch (0 for no limit)")
	imagesCmd.Flags().StringVarP(&imagesOutputDirFlag, "output-dir", "o", "", "Directory to save images (default: [SavePath]/images).")
	// Link to package-level variable
	imagesCmd.Flags().IntVarP(&imagesConcurrencyFlag, "concurrency", "c", 4, "Number of concurrent image downloads")
	// Add the save-metadata flag
	imagesCmd.Flags().BoolVar(&imagesMetadataFlag, "metadata", false, "Save a .json metadata file alongside each downloaded image.")
	// Add the disable-image-mime flag (default false; presence disables MIME detection)
	imagesCmd.Flags().BoolVar(&imagesDisableImageMimeFlag, "disable-image-mime", false, "Disable MIME type detection; keep original URL-derived file extensions")
	// Add the browsing-level flag for precise Civitai content filtering (bitmask: 1=PG, 3=SFW, 31=All)
	imagesCmd.Flags().IntVar(&imagesBrowsingLevelFlag, "browsing-level", 0, "Civitai browsing level bitmask (1=PG, 3=SFW, 31=All). Overrides --nsfw when set.")

	// Hidden flag for testing API URL generation
	imagesCmd.Flags().Bool("debug-print-api-url", false, "Print the constructed API URL for image fetching and exit")
	_ = imagesCmd.Flags().MarkHidden("debug-print-api-url") // Hide from help output
}
