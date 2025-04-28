package cmd

import (
	"fmt"

	"go-civitai-download/internal/models"

	log "github.com/sirupsen/logrus"
)

// Variable to store concurrency level for flag parsing
// var concurrencyLevel int

// Allowed values for API parameters
var allowedSortOrders = map[string]bool{
	"Highest Rated":   true,
	"Most Downloaded": true,
	"Newest":          true,
}

var allowedPeriods = map[string]bool{
	"AllTime": true,
	"Year":    true,
	"Month":   true,
	"Week":    true,
	"Day":     true,
}

// buildQueryParameters initializes the query parameters based on the final loaded config.
// It no longer uses Viper.
func buildQueryParameters(cfg *models.Config) models.QueryParameters {

	// Determine API page limit based on user's total limit preference
	userLimit := cfg.Download.Limit
	apiPageLimit := 100 // Default API page limit
	if userLimit > 0 && userLimit < 100 {
		log.Debugf("User limit (%d) is less than 100, using it for API page limit.", userLimit)
		apiPageLimit = userLimit
	} else if userLimit <= 0 {
		log.Debugf("User limit (%d) is invalid or zero, using default API page limit 100.", userLimit)
		// apiPageLimit remains 100
	} else { // userLimit >= 100
		log.Debugf("User limit (%d) is 100 or greater, using default API page limit 100 for efficiency.", userLimit)
		// apiPageLimit remains 100
	}

	sort := cfg.Download.Sort
	if _, ok := allowedSortOrders[sort]; !ok {
		log.Warnf("Invalid Sort value '%s' from config/flags, using default 'Most Downloaded'", sort)
		sort = "Most Downloaded" // Default sort
	}

	period := cfg.Download.Period
	if _, ok := allowedPeriods[period]; !ok {
		log.Warnf("Invalid Period value '%s' from config/flags, using default 'AllTime'", period)
		period = "AllTime" // Default period
	}

	// Handle Username vs Usernames mismatch
	// Use the first username if the list is not empty, otherwise empty string
	username := ""
	if len(cfg.Download.Usernames) > 0 {
		if len(cfg.Download.Usernames) > 1 {
			log.Warnf("Multiple usernames found in config Usernames list (%v), but API/flag only supports one. Using the first: '%s'", cfg.Download.Usernames, cfg.Download.Usernames[0])
		}
		username = cfg.Download.Usernames[0]
	}

	params := models.QueryParameters{
		Limit:           apiPageLimit, // Use the calculated API page limit
		Page:            1,            // Page is usually not used with cursor-based pagination
		Query:           cfg.Download.Query,
		Tag:             cfg.Download.Tag,
		Username:        username, // Use derived single username
		Types:           cfg.Download.ModelTypes,
		Sort:            sort,
		Period:          period,
		PrimaryFileOnly: cfg.Download.PrimaryOnly,
		// Defaults for fields not typically overridden by user flags/config
		AllowNoCredit:          true,
		AllowDerivatives:       true,
		AllowDifferentLicenses: true,
		AllowCommercialUse:     "Any",
		Nsfw:                   cfg.Download.Nsfw,
		BaseModels:             cfg.Download.BaseModels,
	}

	log.WithField("params", fmt.Sprintf("%+v", params)).Debug("Final query parameters constructed")
	return params
}
