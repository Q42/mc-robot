package servicesync

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func keys(m interface{}) []string {
	mp, isMap := m.(map[string]interface{})
	if !isMap {
		return []string{}
	}
	keys := make([]string, 0, len(mp))
	for key := range mp {
		keys = append(keys, key)
	}
	return keys
}

func orElse(a, b interface{}) interface{} {
	if a == nil {
		return b
	}
	return a
}
