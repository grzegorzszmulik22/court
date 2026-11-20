package auth

const SessionTTLSeconds = 3600

func Valid(ttl int) bool {
	return ttl > 0 && ttl <= SessionTTLSeconds
}
