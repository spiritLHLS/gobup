package upload

// bv2avLocal 将 BV 号转换为 AV 号（upload 包内部使用，避免跨包循环导入）
func bv2avLocal(bv string) int64 {
	const (
		xorCode  = int64(23442827791579)
		maskCode = int64(2251799813685247)
		base     = 58
		alphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	)
	if len(bv) != 12 || bv[:2] != "BV" {
		return 0
	}
	charMap := make(map[byte]int64)
	for i, c := range alphabet {
		charMap[byte(c)] = int64(i)
	}
	bytes := []byte(bv)
	bytes[3], bytes[9] = bytes[9], bytes[3]
	bytes[4], bytes[7] = bytes[7], bytes[4]
	var tmp int64
	for i := 2; i < len(bytes); i++ {
		tmp = tmp*base + charMap[bytes[i]]
	}
	return (tmp ^ xorCode) & maskCode
}

// Av2Bv 将AV号转换为BV号
// 算法参考: https://github.com/SocialSisterYi/bilibili-API-collect
func Av2Bv(av int64) string {
	const (
		xorCode  = int64(23442827791579)
		maskCode = int64(2251799813685247)
		maxAid   = int64(1) << 51
		base     = 58
		alphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	)

	bytes := []byte{'B', 'V', '1', '0', '0', '0', '0', '0', '0', '0', '0', '0'}
	bvIndex := len(bytes) - 1
	tmp := (maxAid | av) ^ xorCode

	for tmp > 0 {
		bytes[bvIndex] = alphabet[tmp%base]
		tmp /= base
		bvIndex--
	}

	// 交换特定位置的字符
	bytes[3], bytes[9] = bytes[9], bytes[3]
	bytes[4], bytes[7] = bytes[7], bytes[4]

	return string(bytes)
}
