package parser

type TypeName string
type IdName string

type Identifier struct {
    Type TypeName
    Name IdName
}

type ProcNode struct {
    Name IdName
    Args []Identifier
    Ret TypeName
    Body []interface{}
}
