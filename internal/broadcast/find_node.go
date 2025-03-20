package broadcast

func CreateSubTree(left int, right int, current int, n int, k int, coloring bool) ([]*area, int) {
	offset := 0
	//偏移到正数方便算
	if left > right {
		offset = left
		current = ObtainOnIPRing(current, -offset, n)
		right = ObtainOnIPRing(right, -offset, n)
		left = 0
	}
	tree := make([]*area, 0)
	//是否进行节点染色

	if coloring {
		tree = ColoringMultiwayTree(left, right, current, k)
	} else {
		tree = BalancedMultiwayTree(left, right, current, k)
	}

	areaLen := right - left + 1
	//计算完把偏移设置为原位
	for _, v := range tree {
		v.current = ObtainOnIPRing(v.current, offset, n)
		v.right = ObtainOnIPRing(v.right, offset, n)
		v.left = ObtainOnIPRing(v.left, offset, n)

	}

	return tree, areaLen
}

func BalancedMultiwayTree(left int, right int, current int, k int) []*area {

	AreaLen := right - left + 1
	areas := make([]*area, 0)
	//除去自己的，剩余节点小于等于k，那就直接转发
	if left > right {
		return nil
	} else if (AreaLen - 1) <= k {
		for ; left <= right; left++ {
			if left == current {
				continue
			}
			//只有这一个节点了
			areas = append(areas, &area{
				current: left,
				left:    left,
				right:   left,
			})
		}
		return areas
	}
	leftArea := (current - left) / (k / 2)
	leftRemain := (current - left) % (k / 2)
	previousScope := left
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := leftArea
		if leftRemain > 0 {
			leftRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		leftNodeValue := (previousScope + (rightBound + 1)) / 2
		areas = append(areas, &area{left: previousScope, right: rightBound, current: leftNodeValue})
		previousScope = rightBound + 1
	}

	previousScope = current + 1
	rightArea := (right - current) / (k / 2)
	rightRemain := (right - current) % (k / 2)
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := rightArea
		if rightRemain > 0 {
			rightRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		rightNodeValue := (previousScope + (rightBound + 1)) / 2
		areas = append(areas, &area{left: previousScope, right: rightBound, current: rightNodeValue})

		previousScope = rightBound + 1
	}

	return areas
}

func ColoringMultiwayTree(left int, right int, current int, k int) []*area {
	AreaLen := right - left + 1
	areas := make([]*area, 0)
	//除去自己的，剩余节点小于等于k，那就直接转发
	if left > right {
		return nil
	} else if (AreaLen - 1) <= k {
		//for ; left < current; left++ {
		//	areas = append(areas, &area{left: left, current: left, right: left})
		//}
		//for ; current < right; current++ {
		//	current++
		//	areas = append(areas, &area{left: current, current: current, right: current})
		//}
		//return areas
		for ; left <= right; left++ {
			if left == current {
				continue
			}
			//只有这一个节点了
			areas = append(areas, &area{
				current: left,
				left:    left,
				right:   left,
			})
		}
		return areas
	}
	//当前找单数还是双数
	parity := current % 2
	leftArea := (current - left) / (k / 2)
	leftRemain := (current - left) % (k / 2)
	previousScope := left
	for i := 0; i < k/2; i++ {
		if previousScope == current {
			continue
		}
		//将多余的区域从左边开始均分给每一个节点
		currentArea := leftArea
		if leftRemain > 0 {
			leftRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		leftNodeValue := (previousScope + (rightBound + 1)) / 2
		//先找到和当前单双数一样的节点,如果找不到的化就直接返回当前值
		if leftNodeValue%2 != parity {
			if leftNodeValue < rightBound {
				leftNodeValue++
			} else if leftNodeValue > previousScope {
				leftNodeValue--
			}
		}
		if leftNodeValue != current {
			areas = append(areas, &area{left: previousScope, right: rightBound, current: leftNodeValue})
		}
		previousScope = rightBound + 1
	}

	if previousScope >= right {
		return areas
	}
	previousScope = current + 1

	rightArea := (right - current) / (k / 2)
	rightRemain := (right - current) % (k / 2)
	for i := 0; i < k/2; i++ {
		if previousScope > right {
			continue
		}
		//将多余的区域从左边开始均分给每一个节点
		currentArea := rightArea
		if rightRemain > 0 {
			rightRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		rightNodeValue := (previousScope + (rightBound + 1)) / 2
		//先找到和当前单双数一样的节点

		if rightNodeValue%2 != parity {
			if rightNodeValue < rightBound {
				rightNodeValue++
			} else if rightNodeValue > previousScope {
				rightNodeValue--
			}
		}
		if rightNodeValue != current {
			areas = append(areas, &area{left: previousScope, right: rightBound, current: rightNodeValue})
		}
		previousScope = rightBound + 1
	}

	return areas

}
