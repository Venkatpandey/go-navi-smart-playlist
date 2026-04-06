package similarity

import "math"

func CosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return 0
	}

	var dotProduct float64
	var leftNorm float64
	var rightNorm float64
	for index := range left {
		dotProduct += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}

	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func Centroid(vectors [][]float64) []float64 {
	if len(vectors) == 0 {
		return nil
	}

	width := len(vectors[0])
	centroid := make([]float64, width)
	for _, vector := range vectors {
		if len(vector) != width {
			return nil
		}
		for index, value := range vector {
			centroid[index] += value
		}
	}

	for index := range centroid {
		centroid[index] /= float64(len(vectors))
	}

	return centroid
}
