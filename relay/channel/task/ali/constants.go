package ali

var ModelList = []string{
	// wan2.7 系列 (新格式 - media 数组)
	"wan2.7-i2v",
	"wan2.7-t2v",
	"wan2.7-r2v",
	"wan2.7-videoedit",
	"wan2.7-vace",

	// wan2.6 系列 (旧格式 - 扁平字段)
	"wan2.6-i2v-flash",
	"wan2.6-i2v",
	"wan2.6-t2v",
	"wan2.6-r2v-flash",
	"wan2.6-r2v",

	// wan2.5 系列 (旧格式)
	"wan2.5-t2v-preview",
	"wan2.5-i2v-preview",

	// wan2.2 系列 (旧格式)
	"wan2.2-t2v-plus",
	"wan2.2-i2v-flash",
	"wan2.2-i2v-plus",
	"wan2.2-kf2v-flash",

	// wanx2.1 系列 (旧格式)
	"wanx2.1-t2v-turbo",
	"wanx2.1-t2v-plus",
	"wanx2.1-i2v-turbo",
	"wanx2.1-i2v-plus",
	"wanx2.1-kf2v-plus",
	"wanx2.1-vace-plus",

	// happyhorse 系列 (新格式 - media 数组)
	"happyhorse-1.0-t2v",
	"happyhorse-1.0-i2v",
	"happyhorse-1.0-r2v",
	"happyhorse-1.0-video-edit",
}

var ChannelName = "ali"