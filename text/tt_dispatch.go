// TrueType bytecode interpreter — opcode dispatch.
//
// Port of skrifa hint/engine/dispatch.rs (243 LOC).
// Maps all ~200 TrueType opcodes to their handler functions.
//
// Reference: skrifa/src/outline/glyf/hint/engine/dispatch.rs
package text

// TrueType opcode constants.
// Reference: OpenType spec, TrueType instruction set.
const (
	opSVTCA0   = 0x00 // Set freedom & projection vectors to Y axis
	opSVTCA1   = 0x01 // Set freedom & projection vectors to X axis
	opSPVTCA0  = 0x02 // Set projection vector to Y axis
	opSPVTCA1  = 0x03 // Set projection vector to X axis
	opSFVTCA0  = 0x04 // Set freedom vector to Y axis
	opSFVTCA1  = 0x05 // Set freedom vector to X axis
	opSPVTL0   = 0x06 // Set projection vector to line (parallel)
	opSPVTL1   = 0x07 // Set projection vector to line (perpendicular)
	opSFVTL0   = 0x08 // Set freedom vector to line (parallel)
	opSFVTL1   = 0x09 // Set freedom vector to line (perpendicular)
	opSPVFS    = 0x0A // Set projection vector from stack
	opSFVFS    = 0x0B // Set freedom vector from stack
	opGPV      = 0x0C // Get projection vector
	opGFV      = 0x0D // Get freedom vector
	opSFVTPV   = 0x0E // Set freedom vector to projection vector
	opISECT    = 0x0F // Move point to intersection
	opSRP0     = 0x10 // Set reference point 0
	opSRP1     = 0x11 // Set reference point 1
	opSRP2     = 0x12 // Set reference point 2
	opSZP0     = 0x13 // Set zone pointer 0
	opSZP1     = 0x14 // Set zone pointer 1
	opSZP2     = 0x15 // Set zone pointer 2
	opSZPS     = 0x16 // Set all zone pointers
	opSLOOP    = 0x17 // Set loop counter
	opRTG      = 0x18 // Round to grid
	opRTHG     = 0x19 // Round to half grid
	opSMD      = 0x1A // Set minimum distance
	opELSE     = 0x1B // Else clause
	opJMPR     = 0x1C // Jump relative
	opSCVTCI   = 0x1D // Set CVT cutin
	opSSWCI    = 0x1E // Set single width cutin
	opSSW      = 0x1F // Set single width
	opDUP      = 0x20 // Duplicate top stack element
	opPOP      = 0x21 // Pop top stack element
	opCLEAR    = 0x22 // Clear the entire stack
	opSWAP     = 0x23 // Swap top two elements
	opDEPTH    = 0x24 // Push stack depth
	opCINDEX   = 0x25 // Copy indexed element
	opMINDEX   = 0x26 // Move indexed element
	opALIGNPTS = 0x27 // Align points
	// 0x28 unused
	opUTP      = 0x29 // Untouch point
	opLOOPCALL = 0x2A // Loop and call function
	opCALL     = 0x2B // Call function
	opFDEF     = 0x2C // Function definition
	opENDF     = 0x2D // End function definition
	opMDAP0    = 0x2E // Move direct absolute point (no round)
	opMDAP1    = 0x2F // Move direct absolute point (round)
	opIUP0     = 0x30 // Interpolate untouched points (Y)
	opIUP1     = 0x31 // Interpolate untouched points (X)
	opSHP0     = 0x32 // Shift point using rp2
	opSHP1     = 0x33 // Shift point using rp1
	opSHC0     = 0x34 // Shift contour using rp2
	opSHC1     = 0x35 // Shift contour using rp1
	opSHZ0     = 0x36 // Shift zone using rp2
	opSHZ1     = 0x37 // Shift zone using rp1
	opSHPIX    = 0x38 // Shift point by pixel amount
	opIP       = 0x39 // Interpolate point
	opMSIRP0   = 0x3A // Move stack indirect relative point (no set rp0)
	opMSIRP1   = 0x3B // Move stack indirect relative point (set rp0)
	opALIGNRP  = 0x3C // Align to reference point
	opRTDG     = 0x3D // Round to double grid
	opMIAP0    = 0x3E // Move indirect absolute point (no round)
	opMIAP1    = 0x3F // Move indirect absolute point (round)
	opNPUSHB   = 0x40 // Push N bytes
	opNPUSHW   = 0x41 // Push N words
	opWS       = 0x42 // Write store
	opRS       = 0x43 // Read store
	opWCVTP    = 0x44 // Write CVT in pixel units
	opRCVT     = 0x45 // Read CVT
	opGC0      = 0x46 // Get coordinate (current)
	opGC1      = 0x47 // Get coordinate (original)
	opSCFS     = 0x48 // Set coordinate from stack
	opMD0      = 0x49 // Measure distance (current)
	opMD1      = 0x4A // Measure distance (original)
	opMPPEM    = 0x4B // Measure pixels per em
	opMPS      = 0x4C // Measure point size
	opFLIPON   = 0x4D // Set auto flip on
	opFLIPOFF  = 0x4E // Set auto flip off
	opDEBUG    = 0x4F // Debug (no-op)
	opLT       = 0x50 // Less than
	opLTEQ     = 0x51 // Less than or equal
	opGT       = 0x52 // Greater than
	opGTEQ     = 0x53 // Greater than or equal
	opEQ       = 0x54 // Equal
	opNEQ      = 0x55 // Not equal
	opODD      = 0x56 // Odd
	opEVEN     = 0x57 // Even
	opIF       = 0x58 // If test
	opEIF      = 0x59 // End if
	opAND      = 0x5A // Logical and
	opOR       = 0x5B // Logical or
	opNOT      = 0x5C // Logical not
	opDELTAP1  = 0x5D // Delta exception P1
	opSDB      = 0x5E // Set delta base
	opSDS      = 0x5F // Set delta shift
	opADD      = 0x60 // Add
	opSUB      = 0x61 // Subtract
	opDIV      = 0x62 // Divide
	opMUL      = 0x63 // Multiply
	opABS      = 0x64 // Absolute value
	opNEG      = 0x65 // Negate
	opFLOOR    = 0x66 // Floor
	opCEILING  = 0x67 // Ceiling
	opROUND00  = 0x68 // Round (gray)
	opROUND01  = 0x69 // Round (black)
	opROUND10  = 0x6A // Round (white)
	opROUND11  = 0x6B // Round (reserved)
	opNROUND00 = 0x6C // No round (gray)
	opNROUND01 = 0x6D // No round (black)
	opNROUND10 = 0x6E // No round (white)
	opNROUND11 = 0x6F // No round (reserved)
	opWCVTF    = 0x70 // Write CVT in font units
	opDELTAP2  = 0x71 // Delta exception P2
	opDELTAP3  = 0x72 // Delta exception P3
	opDELTAC1  = 0x73 // Delta exception C1
	opDELTAC2  = 0x74 // Delta exception C2
	opDELTAC3  = 0x75 // Delta exception C3
	opSROUND   = 0x76 // Super round
	opS45ROUND = 0x77 // Super 45 degree round
	opJROT     = 0x78 // Jump relative on true
	opJROF     = 0x79 // Jump relative on false
	opROFF     = 0x7A // Round off
	// 0x7B unused
	opRUTG      = 0x7C // Round up to grid
	opRDTG      = 0x7D // Round down to grid
	opSANGW     = 0x7E // Set angle weight (no-op)
	opAA        = 0x7F // Adjust angle (no-op)
	opFLIPPT    = 0x80 // Flip point on-curve flag
	opFLIPRGON  = 0x81 // Flip range on
	opFLIPRGOFF = 0x82 // Flip range off
	// 0x83, 0x84 unused
	opSCANCTRL = 0x85 // Scan conversion control
	opSDPVTL0  = 0x86 // Set dual projection vector to line (parallel)
	opSDPVTL1  = 0x87 // Set dual projection vector to line (perpendicular)
	opGETINFO  = 0x88 // Get info
	opIDEF     = 0x89 // Instruction definition
	opROLL     = 0x8A // Roll top 3 stack elements
	opMAX      = 0x8B // Maximum
	opMIN      = 0x8C // Minimum
	opSCANTYPE = 0x8D // Set scan type
	opINSTCTRL = 0x8E // Instruction control
	// 0x8F, 0x90 unused
	opGETVARIATION = 0x91 // Get variation coordinates
	opGETDATA      = 0x92 // Get data (no-op, returns 17)
	// 0x93-0xAF unused
	opPUSHB000 = 0xB0 // Push 1 byte
	// 0xB0-0xB7 PUSHB[0-7]
	opPUSHW000 = 0xB8 // Push 1 word
	// 0xB8-0xBF PUSHW[0-7]
	opMDRP00000 = 0xC0 // Move direct relative point (base)
	// 0xC0-0xDF MDRP[32 variants]
	opMIRP00000 = 0xE0 // Move indirect relative point (base)
	// 0xE0-0xFF MIRP[32 variants]
)

// dispatch executes the handler for the given opcode.
// Reference: skrifa hint/engine/dispatch.rs:98-243
func (e *ttEngine) dispatch(opcode byte) error {
	switch {
	// Vector setting (0x00-0x0E)
	case opcode >= opSVTCA0 && opcode <= opSFVTCA1:
		return e.opSvtca(opcode)
	case opcode >= opSPVTL0 && opcode <= opSFVTL1:
		return e.opSvtl(opcode)
	case opcode == opSPVFS:
		return e.opSpvfs()
	case opcode == opSFVFS:
		return e.opSfvfs()
	case opcode == opGPV:
		return e.opGpv()
	case opcode == opGFV:
		return e.opGfv()
	case opcode == opSFVTPV:
		return e.opSfvtpv()
	case opcode == opISECT:
		return e.opIsect()

	// Reference points and zone pointers (0x10-0x17)
	case opcode == opSRP0:
		return e.opSrp0()
	case opcode == opSRP1:
		return e.opSrp1()
	case opcode == opSRP2:
		return e.opSrp2()
	case opcode == opSZP0:
		return e.opSzp0()
	case opcode == opSZP1:
		return e.opSzp1()
	case opcode == opSZP2:
		return e.opSzp2()
	case opcode == opSZPS:
		return e.opSzps()
	case opcode == opSLOOP:
		return e.opSloop()

	// Rounding and distances (0x18-0x1F)
	case opcode == opRTG:
		return e.opRtg()
	case opcode == opRTHG:
		return e.opRthg()
	case opcode == opSMD:
		return e.opSmd()
	case opcode == opELSE:
		return e.opElse()
	case opcode == opJMPR:
		return e.opJmpr()
	case opcode == opSCVTCI:
		return e.opScvtci()
	case opcode == opSSWCI:
		return e.opSswci()
	case opcode == opSSW:
		return e.opSsw()

	// Stack management (0x20-0x27)
	case opcode == opDUP:
		return e.opDup()
	case opcode == opPOP:
		return e.opPop()
	case opcode == opCLEAR:
		return e.opClear()
	case opcode == opSWAP:
		return e.opSwap()
	case opcode == opDEPTH:
		return e.opDepth()
	case opcode == opCINDEX:
		return e.opCindex()
	case opcode == opMINDEX:
		return e.opMindex()
	case opcode == opALIGNPTS:
		return e.opAlignpts()

	// 0x28 unused
	case opcode == opUTP:
		return e.opUtp()

	// Function calls (0x2A-0x2D)
	case opcode == opLOOPCALL:
		return e.opLoopcall()
	case opcode == opCALL:
		return e.opCall()
	case opcode == opFDEF:
		return e.opFdef()
	case opcode == opENDF:
		return e.opEndf()

	// Point movement (0x2E-0x3F)
	case opcode == opMDAP0 || opcode == opMDAP1:
		return e.opMdap(opcode)
	case opcode == opIUP0 || opcode == opIUP1:
		return e.opIup(opcode)
	case opcode == opSHP0 || opcode == opSHP1:
		return e.opShp(opcode)
	case opcode == opSHC0 || opcode == opSHC1:
		return e.opShc(opcode)
	case opcode == opSHZ0 || opcode == opSHZ1:
		return e.opShz(opcode)
	case opcode == opSHPIX:
		return e.opShpix()
	case opcode == opIP:
		return e.opIP()
	case opcode == opMSIRP0 || opcode == opMSIRP1:
		return e.opMsirp(opcode)
	case opcode == opALIGNRP:
		return e.opAlignrp()
	case opcode == opRTDG:
		return e.opRtdg()
	case opcode == opMIAP0 || opcode == opMIAP1:
		return e.opMiap(opcode)

	// Push data (0x40-0x41)
	case opcode == opNPUSHB:
		return e.opNpushb()
	case opcode == opNPUSHW:
		return e.opNpushw()

	// Storage and CVT (0x42-0x4C)
	case opcode == opWS:
		return e.opWs()
	case opcode == opRS:
		return e.opRs()
	case opcode == opWCVTP:
		return e.opWcvtp()
	case opcode == opRCVT:
		return e.opRcvt()
	case opcode == opGC0 || opcode == opGC1:
		return e.opGc(opcode)
	case opcode == opSCFS:
		return e.opScfs()
	case opcode == opMD0 || opcode == opMD1:
		return e.opMd(opcode)
	case opcode == opMPPEM:
		return e.opMppem()
	case opcode == opMPS:
		return e.opMps()

	// Auto flip (0x4D-0x4F)
	case opcode == opFLIPON:
		return e.opFlipon()
	case opcode == opFLIPOFF:
		return e.opFlipoff()
	case opcode == opDEBUG:
		_, err := e.valueStack.pop()
		return err

	// Logical and comparison (0x50-0x5C)
	case opcode == opLT:
		return e.opLt()
	case opcode == opLTEQ:
		return e.opLteq()
	case opcode == opGT:
		return e.opGt()
	case opcode == opGTEQ:
		return e.opGteq()
	case opcode == opEQ:
		return e.opEq()
	case opcode == opNEQ:
		return e.opNeq()
	case opcode == opODD:
		return e.opOdd()
	case opcode == opEVEN:
		return e.opEven()
	case opcode == opIF:
		return e.opIf()
	case opcode == opEIF:
		return nil // End if — no-op
	case opcode == opAND:
		return e.opAnd()
	case opcode == opOR:
		return e.opOr()
	case opcode == opNOT:
		return e.opNot()

	// Delta exceptions (0x5D-0x5F)
	case opcode == opDELTAP1:
		return e.opDeltap(opcode)
	case opcode == opSDB:
		return e.opSdb()
	case opcode == opSDS:
		return e.opSds()

	// Arithmetic (0x60-0x67)
	case opcode == opADD:
		return e.opAdd()
	case opcode == opSUB:
		return e.opSub()
	case opcode == opDIV:
		return e.opDiv()
	case opcode == opMUL:
		return e.opMul()
	case opcode == opABS:
		return e.opAbs()
	case opcode == opNEG:
		return e.opNeg()
	case opcode == opFLOOR:
		return e.opFloor()
	case opcode == opCEILING:
		return e.opCeiling()

	// Round/NRound (0x68-0x6F)
	case opcode >= opROUND00 && opcode <= opROUND11:
		return e.opRound()
	case opcode >= opNROUND00 && opcode <= opNROUND11:
		return nil // NROUND — no-op

	// CVT and more deltas (0x70-0x75)
	case opcode == opWCVTF:
		return e.opWcvtf()
	case opcode == opDELTAP2 || opcode == opDELTAP3:
		return e.opDeltap(opcode)
	case opcode >= opDELTAC1 && opcode <= opDELTAC3:
		return e.opDeltac(opcode)

	// Super round and jumps (0x76-0x7A)
	case opcode == opSROUND:
		return e.opSround()
	case opcode == opS45ROUND:
		return e.opS45round()
	case opcode == opJROT:
		return e.opJrot()
	case opcode == opJROF:
		return e.opJrof()
	case opcode == opROFF:
		return e.opRoff()

	// 0x7B unused

	// More rounding (0x7C-0x7D)
	case opcode == opRUTG:
		return e.opRutg()
	case opcode == opRDTG:
		return e.opRdtg()

	// No-ops (0x7E-0x7F)
	case opcode == opSANGW:
		return e.opSangw()
	case opcode == opAA:
		return nil // AA — no-op

	// Flip operations (0x80-0x82)
	case opcode == opFLIPPT:
		return e.opFlippt()
	case opcode == opFLIPRGON:
		return e.opFliprgon()
	case opcode == opFLIPRGOFF:
		return e.opFliprgoff()

	// 0x83, 0x84 unused

	// Scan and info (0x85-0x8E)
	case opcode == opSCANCTRL:
		return e.opScanctrl()
	case opcode == opSDPVTL0 || opcode == opSDPVTL1:
		return e.opSdpvtl(opcode)
	case opcode == opGETINFO:
		return e.opGetinfo()
	case opcode == opIDEF:
		return e.opIdef()
	case opcode == opROLL:
		return e.opRoll()
	case opcode == opMAX:
		return e.opMax()
	case opcode == opMIN:
		return e.opMin()
	case opcode == opSCANTYPE:
		return e.opScantype()
	case opcode == opINSTCTRL:
		return e.opInstctrl()

	// Variation (0x91-0x92)
	case opcode == opGETVARIATION:
		return e.opGetvariation()
	case opcode == opGETDATA:
		return e.opGetdata()

	// PUSHB[0-7] (0xB0-0xB7)
	case opcode >= opPUSHB000 && opcode <= 0xB7:
		count := int(opcode-opPUSHB000) + 1
		return e.opPushBytes(count)

	// PUSHW[0-7] (0xB8-0xBF)
	case opcode >= opPUSHW000 && opcode <= 0xBF:
		count := int(opcode-opPUSHW000) + 1
		return e.opPushWords(count)

	// MDRP[32 variants] (0xC0-0xDF)
	case opcode >= opMDRP00000 && opcode <= 0xDF:
		return e.opMdrp(opcode)

	// MIRP[32 variants] (0xE0-0xFF)
	case opcode >= opMIRP00000:
		return e.opMirp(opcode)

	default:
		return e.opUnknown(opcode)
	}
}
