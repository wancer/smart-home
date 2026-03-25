package event

type Timezone string

func NewTimezone(payload string) Timezone {
	return Timezone(payload)
}
