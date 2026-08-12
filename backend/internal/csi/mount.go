package csi

import (
	"bufio"
	"os"
	gopath "path"
	"strings"
)

// mountInfoPath는 커널 마운트 테이블. 테스트에서 갈아끼울 수 있게 변수로 둔다.
var mountInfoPath = "/proc/self/mountinfo"

// IsMountPoint는 절대경로가 마운트 지점인지 본다.
//
// os.Stat 의 st_dev 비교(부모와 다르면 마운트)는 bind 마운트를 놓치므로 쓰지 않고
// 커널 마운트 테이블을 직접 읽는다. 마운트 지점 필드는 mountinfo 의 5번째 칼럼이며
// 공백은 8진 이스케이프(\040)로 인코딩돼 있다. 세션 볼륨 이름에 공백이 들어갈 수 있어
// (실제로 "테스트 볼륨" 같은 이름이 있다) 디코딩이 필요하다.
//
// 경로 정규화는 filepath 가 아니라 path 로 한다. 드라이버는 리눅스에서만 돌지만
// 테스트는 다른 OS 에서도 돌아야 하는데, filepath 는 구분자와 절대경로 판정이
// OS 마다 달라 "/proc" 이 "C:\proc" 이 되는 식으로 어긋난다.
func IsMountPoint(p string) (bool, error) {
	abs := gopath.Clean(p)
	f, err := os.Open(mountInfoPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		if unescapeMountField(fields[4]) == abs {
			return true, nil
		}
	}
	return false, sc.Err()
}

// unescapeMountField는 mountinfo 의 8진 이스케이프(\040 공백, \011 탭, \012 개행, \134 역슬래시)를 되돌린다.
func unescapeMountField(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}
