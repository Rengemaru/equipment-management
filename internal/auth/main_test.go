package auth

import (
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// TestMain は bcrypt のコストを最小に落としてからテストを走らせる。
//
// このパッケージのテストは何十回もハッシュ化する。既定のコスト(10)のままだと
// -race 込みで分単位になり、CI で毎push回すには重すぎる。
//
// コストを下げてもハッシュ化・照合の経路は変わらないため、
// 検証したいこと（平文を保存しない、照合が通る、ソルトが効く）は落ちない。
func TestMain(m *testing.M) {
	hashCost = bcrypt.MinCost

	// 総当たり対策の待ち時間も刻みを縮める。1秒刻みのままだと、
	// 失敗を重ねるテストが数十秒かかる。
	delayUnit = time.Millisecond

	os.Exit(m.Run())
}
