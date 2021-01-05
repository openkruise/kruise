package filters

import (
	"encoding/json"
	"fmt"
	"strings"
)

// filter.go is used for listing container with filter conditions

// acceptedFilters defines filter key ps support
var acceptedFilters = map[string]bool{
	"id":     true,
	"label":  true,
	"name":   true,
	"status": true,

	/*
		// TODO(huamin.thm): the following list key should also support
		"before":  true,
		"since":   true,
		"exited":  true,
		"volume":  true,
		"network": true,
	*/
}

// getAcceptKeys gets all accepted filter keys
func getAcceptKeys() (list []string) {
	for key := range acceptedFilters {
		list = append(list, key)
	}

	return
}

// Parse parses filter format
func Parse(filter []string) (map[string][]string, error) {
	if len(filter) == 0 {
		return nil, nil
	}

	parsed := make(map[string][]string)
	for _, str := range filter {
		splits := strings.SplitN(str, "=", 2)
		if len(splits) != 2 {
			return nil, fmt.Errorf("Bad format of filter, expected name=value")
		}

		name := splits[0]
		if _, ok := acceptedFilters[name]; !ok {
			return nil, fmt.Errorf("Invalid filter %s, accepted filter key: %v", name, getAcceptKeys())
		}

		if v, exist := parsed[name]; exist {
			parsed[name] = append(v, strings.TrimSpace(splits[1]))
		} else {
			parsed[name] = []string{strings.TrimSpace(splits[1])}
		}
	}

	return parsed, nil
}

// ToURLParam marshals filter as a string, used for url query
func ToURLParam(filter map[string][]string) (string, error) {
	if len(filter) == 0 {
		return "", nil
	}

	rawdata, err := json.Marshal(filter)
	if err != nil {
		return "", fmt.Errorf("failed to marshal filter params %v", filter)
	}

	return string(rawdata), nil
}

// FromURLParam gets filter from url query
func FromURLParam(param string) (map[string][]string, error) {
	if param == "" {
		return nil, nil
	}

	var filter map[string][]string
	err := json.NewDecoder(strings.NewReader(param)).Decode(&filter)
	if err != nil {
		return nil, err
	}

	// params from url may not passed through api, so we need validate here
	return filter, Validate(filter)
}

// Validate validates filter key is accepted
func Validate(filter map[string][]string) error {
	for name := range filter {
		if _, exist := acceptedFilters[name]; !exist {
			return fmt.Errorf("invalid filter %s", name)
		}
	}

	return nil
}
