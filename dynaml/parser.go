package dynaml

import (
	"container/list"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mandelsoft/spiff/debug"
)

type helperNode struct {
}

func (e helperNode) Evaluate(binding Binding, locally bool) (interface{}, EvaluationInfo, bool) {
	panic("not intended to be evaluated")
}

type expressionHelper struct {
	helperNode
	expression Expression
}

type expressionListHelper struct {
	helperNode
	list []Expression
}

type nameListHelper struct {
	helperNode
	list []string
}

type nameHelper struct {
	helperNode
	name string
}

type operationHelper struct {
	helperNode
	op string
}

func lineError(lines, syms, linee, syme int, txt string) string {
	return fmt.Sprintf("parse error near symbol %v - symbol %v: %v", syms, syme, txt)
}

func docError(lines, syms, linee, syme int, txt string) string {
	return fmt.Sprintf("parse error near line %v symbol %v - line %v symbol %v: %v", lines, syms, linee, syme, txt)
}

func (e *parseError) String() string {
	tokens, error := []token32{e.max}, ""
	positions, p := make([]int, 2*len(tokens)), 0
	for _, token := range tokens {
		positions[p], p = int(token.begin), p+1
		positions[p], p = int(token.end), p+1
	}
	translations := translatePositions(e.p.buffer, positions)
	errf := lineError
	if strings.Index(e.p.Buffer, "\n") >= 0 {
		errf = docError
	}
	for _, token := range tokens {
		begin, end := int(token.begin), int(token.end)
		error += errf(
			translations[begin].line, translations[begin].symbol,
			translations[end].line, translations[end].symbol,
			strconv.Quote(string(e.p.buffer[begin:end])))
	}

	return error
}

func Parse(source string, path []string, stubPath []string) (Expression, error) {
	grammar := &DynamlGrammar{Buffer: source}
	grammar.Init()

	err := grammar.Parse()
	if err != nil {
		return nil, errors.New(err.(*parseError).String())
	}

	return buildExpression(grammar, path, stubPath), nil
}

func buildExpression(grammar *DynamlGrammar, path []string, stubPath []string) Expression {
	tokens := &tokenStack{}

	// flags for parsing merge options in merge expression
	// this expression is NOT recursive, therefore single flag variables are sufficient
	replace := false
	required := false
	keyName := ""

	for token := range grammar.Tokens() {
		contents := grammar.Buffer[token.begin:token.end]

		switch token.pegRule {
		case ruleDynaml:
			return tokens.Pop()

		case ruleMarker:
			tokens.Push(newMarkerExpr(contents))
		case ruleSubsequentMarker:
			tokens.Pop()
			tokens.Push(tokens.Pop().(MarkerExpr).add(contents))
		case ruleMarkerExpression:
			tokens.Push(MarkerExpressionExpr{contents, tokens.Pop().(Expression)})
		case ruleMarkedExpression:
			rhs := tokens.Pop()
			if _, ok := rhs.(MarkerExpr); !ok {
				rhs = tokens.Pop().(MarkerExpr).setExpression(rhs)
			}
			tokens.Push(rhs)
		case rulePrefer:
			tokens.Push(PreferExpr{tokens.Pop()})
		case ruleGrouped:
			tokens.Push(GroupedExpr{tokens.Pop()})
		case ruleAuto:
			tokens.Push(AutoExpr{path})
		case ruleMerge:
			replace = false
			required = false
			keyName = ""
		case ruleSimpleMerge:
			debug.Debug("*** rule simple merge\n")
			redirect := !equals(path, stubPath)
			tokens.Push(MergeExpr{stubPath, redirect, replace, replace || required || redirect, keyName})
		case ruleRefMerge:
			debug.Debug("*** rule ref merge\n")
			rhs := tokens.Pop()
			merge := rhs.(ReferenceExpr).Path
			if len(merge) == 1 && merge[0] == "none" {
				merge = []string{}
			}
			tokens.Push(MergeExpr{merge, true, replace, len(merge) > 0, keyName})
		case ruleReplace:
			replace = true
		case ruleRequired:
			required = true
		case ruleOn:
			keyName = tokens.Pop().(nameHelper).name
		case ruleFollowUpRef:
		case ruleReference:
			tokens.Push(ReferenceExpr{strings.Split(contents, ".")})

		case ruleChained:
		case ruleChainedQualifiedExpression:
		case ruleChainedRef:
			ref := ReferenceExpr{strings.Split(contents, ".")}
			expr := tokens.Pop()
			tokens.Push(QualifiedExpr{expr, ref})
		case ruleChainedDynRef:
			ref := tokens.Pop()
			expr := tokens.Pop()
			tokens.Push(DynamicExpr{expr, ref.(Expression)})
		case ruleSlice:
			slice := tokens.Pop()
			expr := tokens.Pop()
			tokens.Push(SliceExpr{expr, slice.(RangeExpr)})

		case ruleChainedCall:
			args := tokens.PopExpressionList()
			f := tokens.Pop()
			tokens.Push(CallExpr{
				Function:  f,
				Arguments: args,
			})

		case ruleAction0:
		case ruleAction1:
		case ruleAction2:

		case ruleProjectionValue:
			value := &ProjectionValue{}
			tokens.Push(ProjectionValueExpr{value})
			tokens.Push(ProjectionValueExpr{value})

		case ruleProjection:
			qual := tokens.Pop()
			value := tokens.Pop()
			expr := tokens.Pop()
			tokens.Push(ProjectionExpr{expr, value.(ProjectionValueExpr).Value, qual})

		case ruleInteger:
			val, err := strconv.ParseInt(contents, 10, 64)
			if err != nil {
				panic(err)
			}

			tokens.Push(IntegerExpr{val})
		case ruleNil:
			tokens.Push(NilExpr{})
		case ruleUndefined:
			tokens.Push(UndefinedExpr{})

		case ruleScoped:
			e := tokens.Pop()
			m := tokens.Pop().(CreateMapExpr)
			tokens.Push(ScopeExpr{m, e})

		case ruleCreateMap, ruleCreateScope:
			tokens.Push(CreateMapExpr{})
		case ruleAssignment:
			rhs := tokens.Pop()
			lhs := tokens.Pop()
			m := tokens.Pop().(CreateMapExpr)
			m.Assignments = append(m.Assignments, Assignment{lhs, rhs})
			tokens.Push(m)

		case ruleBoolean:
			tokens.Push(BooleanExpr{contents == "true"})
		case ruleString:
			val := strings.Replace(contents[1:len(contents)-1], `\"`, `"`, -1)
			tokens.Push(StringExpr{val})
		case ruleIP:
			tokens.Push(StringExpr{contents})
		case ruleSubstitution:
			tokens.Push(SubstitutionExpr{Template: tokens.Pop()})

		case ruleConditional:
			fhs := tokens.Pop()
			ths := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(CondExpr{C: lhs, T: ths, F: fhs})

		case ruleLogOr:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(LogOrExpr{A: lhs, B: rhs})

		case ruleLogAnd:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(LogAndExpr{A: lhs, B: rhs})

		case ruleOr:
			rhs := tokens.Pop()
			op := tokens.Pop()
			lhs := tokens.Pop()

			if op.(operationHelper).op == "||" {
				tokens.Push(OrExpr{A: lhs, B: rhs})
			} else {
				tokens.Push(ValidOrExpr{A: lhs, B: rhs})
			}

		case ruleOrOp:
			tokens.Push(operationHelper{op: contents})

		case ruleNot:
			tokens.Push(NotExpr{tokens.Pop()})

		case ruleCompareOp:
			tokens.Push(operationHelper{op: contents})

		case ruleComparison:
			rhs := tokens.Pop()
			op := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(ComparisonExpr{A: lhs, Op: op.(operationHelper).op, B: rhs})

		case ruleConcatenation:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(ConcatenationExpr{A: lhs, B: rhs})
		case ruleAddition:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(AdditionExpr{A: lhs, B: rhs})
		case ruleSubtraction:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(SubtractionExpr{A: lhs, B: rhs})
		case ruleMultiplication:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(MultiplicationExpr{A: lhs, B: rhs})
		case ruleDivision:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(DivisionExpr{A: lhs, B: rhs})
		case ruleModulo:
			rhs := tokens.Pop()
			lhs := tokens.Pop()

			tokens.Push(ModuloExpr{A: lhs, B: rhs})

		case ruleSymbol:
			name := tokens.Pop().(nameHelper)
			tokens.Push(StringExpr{name.name})

		case ruleStartParams:
			tokens.Push(nameListHelper{})
		case ruleName:
			tokens.Push(nameHelper{name: contents})
		case ruleNextName:
			rhs := tokens.Pop().(nameHelper)
			list := tokens.Pop().(nameListHelper)
			list.list = append(list.list, rhs.name)
			tokens.Push(list)

		case ruleDefault:
			tokens.Push(DefaultExpr{})

		case ruleLambdaOrExpr:
		case ruleLambdaExt:
			tokens.Push(expressionHelper{expression: tokens.Pop()})
		case ruleSync:
			timeout := tokens.Pop()
			value := tokens.Pop()
			cond := tokens.Pop()
			expr := tokens.Pop()
			if h, ok := value.(expressionHelper); ok {
				value = LambdaExpr{E: h.expression, Names: cond.(LambdaExpr).Names}
			}
			tokens.Push(SyncExpr{A: expr, Cond: cond, Value: value, Timeout: timeout})
		case ruleCatch:
			rhs := tokens.Pop()
			lhs := tokens.Pop()
			tokens.Push(CatchExpr{Lambda: rhs, A: lhs})
		case ruleMapping:
			rhs := tokens.Pop()
			lhs := tokens.Pop()
			tokens.Push(MappingExpr{Lambda: rhs, A: lhs, Context: MapToListContext})
		case ruleSelection:
			rhs := tokens.Pop()
			lhs := tokens.Pop()
			tokens.Push(MappingExpr{Lambda: rhs, A: lhs, Context: SelectToListContext})
		case ruleMapMapping:
			rhs := tokens.Pop()
			lhs := tokens.Pop()
			tokens.Push(MappingExpr{Lambda: rhs, A: lhs, Context: MapToMapContext})
		case ruleMapSelection:
			rhs := tokens.Pop()
			lhs := tokens.Pop()
			tokens.Push(MappingExpr{Lambda: rhs, A: lhs, Context: SelectToMapContext})

		case ruleSum:
			rhs := tokens.Pop()
			ini := tokens.Pop()
			lhs := tokens.Pop()
			tokens.Push(SumExpr{Lambda: rhs, A: lhs, I: ini})

		case ruleLambda:

		case ruleLambdaExpr:
			rhs := tokens.Pop()
			names := tokens.Pop().(nameListHelper)
			tokens.Push(LambdaExpr{Names: names.list, E: rhs})

		case ruleLambdaRef:
			rhs := tokens.Pop()
			lexp, ok := rhs.(LambdaExpr)
			if ok {
				tokens.Push(lexp)
			} else {
				tokens.Push(LambdaRefExpr{Source: rhs, Path: path, StubPath: stubPath})
			}

		case ruleStartRange:
			tokens.Push(operationHelper{op: ""})
		case ruleRangeOp:
			tokens.Push(operationHelper{op: contents})

		case ruleRange:
			rhs := tokens.Pop()
			if _, ok := rhs.(operationHelper); ok {
				rhs = nil
			} else {
				tokens.Pop()
			}
			lhs := tokens.Pop()
			if _, ok := lhs.(operationHelper); ok {
				lhs = nil
			} else {
				tokens.Pop()
			}
			tokens.Push(RangeExpr{lhs, rhs})

		case ruleList:
			seq := tokens.PopExpressionList()
			tokens.Push(ListExpr{seq})

		case ruleNextExpression:
			rhs := tokens.Pop()
			list := tokens.Pop().(expressionListHelper)
			list.list = append(list.list, rhs)
			tokens.Push(list)

		case ruleStartList, ruleStartArguments:
			tokens.Push(expressionListHelper{})

		case ruleKey, ruleIndex:
		case ruleLevel0, ruleLevel1, ruleLevel2, ruleLevel3, ruleLevel4, ruleLevel5, ruleLevel6, ruleLevel7:
		case ruleExpression:
		case ruleExpressionList:
		case ruleMap:
		case ruleScope:
		case ruleAssignments:
		case ruleNames:
		case ruleParams:
		case rulews:
		case rulereq_ws:
		default:
			panic("unhandled:" + rul3s[token.pegRule])
		}
	}

	panic("unreachable")
}

func reverse(a []string) {
	for i := 0; i < len(a)/2; i++ {
		a[i], a[len(a)-i-1] = a[len(a)-i-1], a[i]
	}
}

func equals(p1 []string, p2 []string) bool {
	if len(p1) != len(p2) {
		return false
	}
	for i := 0; i < len(p1); i++ {
		if p1[i] != p2[i] {
			return false
		}
	}
	return true
}

type tokenStack struct {
	list.List
}

func (s *tokenStack) Pop() Expression {
	front := s.Front()
	if front == nil {
		return nil
	}

	s.Remove(front)

	return front.Value.(Expression)
}

func (s *tokenStack) Peek() Expression {
	front := s.Front()
	if front == nil {
		return nil
	}

	return front.Value.(Expression)
}

func (s *tokenStack) Push(expr Expression) {
	s.PushFront(expr)
}

func (s *tokenStack) PopExpressionList() []Expression {
	return (s.Pop().(expressionListHelper)).list
}
