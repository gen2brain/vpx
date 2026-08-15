package vp8

const (
	trellisMinDelta = 0
	trellisDeltas   = trellisMinDelta + 2
	trellisDead     = int64(1) << 55
	trellisDistMul  = 256
)

var trellisWeights = [16]int64{
	16, 16, 16, 16,
	16, 16, 16, 16,
	16, 16, 16, 16,
	16, 16, 16, 16,
}

type trellisNode struct {
	prev  int8
	sign  bool
	level int32
}

type trellisState struct {
	score int64
	ctx   int
}

func levelCost(p *[numProbas]uint8, ctx, level int) int {
	cost := 0

	if ctx > 0 {
		cost += probCost(p[0], 1)
	}

	if level == 0 {
		return cost + probCost(p[1], 0)
	}

	cost += probCost(p[1], 1) + 256

	if level == 1 {
		return cost + probCost(p[2], 0)
	}

	return cost + probCost(p[2], 1) + largeValueCost(p, int32(level))
}

func (e *Encoder) trellisQuantize(in, out []int16, mtx *qmatrix, ty, ctx0, first, lambda int) int {
	bands := &e.proba.bandsPtr[ty]

	var (
		nodes [16][trellisDeltas]trellisNode
		state [2][trellisDeltas]trellisState
	)

	cur, prev := 0, 1

	thresh := int32(mtx.q[1]) * int32(mtx.q[1]) / 4
	last := first - 1

	for n := 15; n >= first; n-- {
		if v := int32(in[zigzag[n]]); v*v > thresh {
			last = n

			break
		}
	}

	if last < 15 {
		last++
	}

	lastProba := bands[first][ctx0][0]

	bestScore := int64(probCost(lastProba, 0)) * int64(lambda)
	bestLast, bestLevel, bestPrev := -1, 0, 0

	entry := 0
	if ctx0 == 0 {
		entry = probCost(lastProba, 1)
	}

	for m := range trellisDeltas {
		state[cur][m] = trellisState{score: int64(entry) * int64(lambda), ctx: ctx0}
	}

	for n := first; n <= last; n++ {
		j := zigzag[n]

		v := int32(in[j])

		sign := v < 0
		if sign {
			v = -v
		}

		coeff := uint32(v) + mtx.sharpen[j]
		scaled := coeff * mtx.iq[j]

		level0 := int32(min(scaled>>qfix, maxLevel))
		levelMax := int32(min((scaled+1<<(qfix-1))>>qfix, maxLevel))

		cur, prev = prev, cur

		for m := range trellisDeltas {
			level := level0 - trellisMinDelta + int32(m)

			ctx := min(int(level), 2)
			state[cur][m].ctx = ctx

			if level < 0 || level > levelMax {
				state[cur][m].score = trellisDead

				continue
			}

			err := int64(coeff) - int64(level)*int64(mtx.q[j])
			delta := trellisWeights[j] * (err*err - int64(coeff)*int64(coeff))

			score := trellisDead
			from := 0

			for p := range trellisDeltas {
				if state[prev][p].score >= trellisDead {
					continue
				}

				pctx := state[prev][p].ctx
				cost := levelCost(&bands[n][pctx], pctx, int(level))

				if s := state[prev][p].score + int64(cost)*int64(lambda); s < score {
					score, from = s, p
				}
			}

			score += trellisDistMul * delta

			nodes[n][m] = trellisNode{prev: int8(from), sign: sign, level: level}
			state[cur][m].score = score

			if level == 0 || score >= bestScore {
				continue
			}

			end := int64(0)
			if n < 15 {
				end = int64(probCost(bands[n+1][ctx][0], 0)) * int64(lambda)
			}

			if s := score + end; s < bestScore {
				bestScore = s
				bestLast, bestLevel, bestPrev = n, m, from
			}
		}
	}

	clear(in[first:16])
	clear(out[first:16])

	if bestLast < 0 {
		return 0
	}

	nodes[bestLast][bestLevel].prev = int8(bestPrev)

	node := bestLevel

	for n := bestLast; n >= first; n-- {
		nd := &nodes[n][node]

		level := nd.level
		if nd.sign {
			level = -level
		}

		out[n] = int16(level)
		in[zigzag[n]] = int16(level * int32(mtx.q[zigzag[n]]))

		node = int(nd.prev)
	}

	return bestLast + 1
}
