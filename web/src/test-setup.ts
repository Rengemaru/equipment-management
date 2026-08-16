import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// テストごとに描画した DOM を片付ける。
//
// vitest の globals を有効にしていないため、Testing Library は自動では
// 後片付けをしない。残ったままだと、次のテストで同じ文言が2つ見つかり、
// 「壊れていないのに落ちる」テストになる。
afterEach(cleanup)
