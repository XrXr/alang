// Code generated by "stringer -type=InstType"; DO NOT EDIT.

package ir

import "strconv"

const _InstType_name = "ZeroVarInstructionsReturnTranscludeJumpStartProcEndProcLabelMutateOnlyInstructionsCallAssignImmIncrementDecrementReadOnlyInstructionsJumpIfFalseJumpIfTrueCompareReadAndMutateInstructionsSubAssignAddMultDivTakeAddressArrayToPointerIndirectWriteIndirectLoadStructMemberPtrLoadStructMemberNotAndOr"

var _InstType_index = [...]uint16{0, 19, 25, 35, 39, 48, 55, 60, 82, 86, 95, 104, 113, 133, 144, 154, 161, 186, 189, 195, 198, 202, 205, 216, 230, 243, 255, 270, 286, 289, 292, 294}

func (i InstType) String() string {
	if i < 0 || i >= InstType(len(_InstType_index)-1) {
		return "InstType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _InstType_name[_InstType_index[i]:_InstType_index[i+1]]
}
