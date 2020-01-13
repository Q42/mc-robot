package servicesync

import "reflect"

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func logOnError(err error, msg string) {
	if err != nil {
		log.Error(err, msg)
	}
}

func keys(m interface{}) []string {
	keys := make([]string, 0)
	v := reflect.ValueOf(m)
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			keys = append(keys, k.String())
		}
	default:
		log.Error(nil, "Invalid value for keys(...)")
	}
	return keys
}

func orElse(a, b interface{}) interface{} {
	if a == nil {
		return b
	}
	return a
}

func filterOut(list []string, needle string) []string {
	result := make([]string, 0)
	for _, item := range list {
		if item != needle {
			result = append(result, needle)
		}
	}
	return result
}
