package parser

import (
	"errors"
	"fmt"
	"go/ast"
	"regexp"
	"strconv"
	"strings"
)

type Operation struct {
	HttpMethod       string            `json:"httpMethod"`
	Nickname         string            `json:"nickname"`
	Type             string            `json:"type"`
	Items            OperationItems    `json:"items,omitempty"`
	Summary          string            `json:"summary,omitempty"`
	Notes            string            `json:"notes,omitempty"`
	Parameters       []Parameter       `json:"parameters,omitempty"`
	ResponseMessages []ResponseMessage `json:"responseMessages,omitempty"`
	Consumes         []string          `json:"consumes,omitempty"`
	Produces         []string          `json:"produces,omitempty"`
	Authorizations   []Authorization   `json:"authorizations,omitempty"`
	Protocols        []Protocol        `json:"protocols,omitempty"`
	Path             string            `json:`
	parser           *Parser
	models           []*Model
	packageName      string
}
type OperationItems struct {
	Ref  string `json:"$ref,omitempty"`
	Type string `json:"type,omitempty"`
}

func NewOperation(p *Parser, packageName string) *Operation {
	return &Operation{
		parser:      p,
		models:      make([]*Model, 0),
		packageName: packageName,
	}
}

func (operation *Operation) SetItemsType(itemsType string) {
	operation.Items = OperationItems{}
	if IsBasicType(itemsType) {
		operation.Items.Type = itemsType
	} else {
		operation.Items.Ref = itemsType
	}
}

func (operation *Operation) ParseComment(commentList *ast.CommentGroup) error {
	if commentList != nil && commentList.List != nil {
		for _, comment := range commentList.List {
			//log.Printf("Parse comemnt: %#v\n", c)
			commentLine := strings.TrimSpace(strings.TrimLeft(comment.Text, "//"))
			if strings.HasPrefix(commentLine, "@router") {
				if err := operation.ParseRouterComment(commentLine); err != nil {
					return err
				}
			} else if strings.HasPrefix(commentLine, "@Title") {
				operation.Nickname = strings.TrimSpace(commentLine[len("@Title"):])
			} else if strings.HasPrefix(commentLine, "@Description") {
				operation.Summary = strings.TrimSpace(commentLine[len("@Description"):])
			} else if strings.HasPrefix(commentLine, "@Success") {
				sourceString := strings.TrimSpace(commentLine[len("@Success"):])
				if err := operation.ParseResponseComment(sourceString); err != nil {
					return err
				}
			} else if strings.HasPrefix(commentLine, "@Param") {
				if err := operation.ParseParamComment(commentLine); err != nil {
					return err
				}
			} else if strings.HasPrefix(commentLine, "@Failure") {
				sourceString := strings.TrimSpace(commentLine[len("@Failure"):])
				if err := operation.ParseResponseComment(sourceString); err != nil {
					return err
				}
			} else if strings.HasPrefix(commentLine, "@Accept") {
				if err := operation.ParseAcceptComment(commentLine); err != nil {
					return err
				}
			}
		}
	} else {
		return CommentIsEmptyError
	}

	if operation.Path == "" {
		return CommentIsEmptyError
	}
	return nil
}

// Parse params return []string of param properties
// @Param	queryText		form	      string	  true		        "The email for login"
// 			[param name]    [param type] [data type]  [is mandatory?]   [Comment]
func (operation *Operation) ParseParamComment(commentLine string) error {
	swaggerParameter := Parameter{}
	paramString := strings.TrimSpace(commentLine[len("@Param "):])

	re := regexp.MustCompile(`([\w]+)[\s]+([\w]+)[\s]+([\w]+)[\s]+([\w]+)[\s]+"([^"]+)"`)

	if matches := re.FindStringSubmatch(paramString); len(matches) != 6 {
		return fmt.Errorf("Can not parse param comment \"%s\", skipped.", paramString)
	} else {
		//TODO: if type is not simple, then add to Models[]
		swaggerParameter.Name = matches[1]
		swaggerParameter.ParamType = matches[2]
		swaggerParameter.Type = matches[3]
		swaggerParameter.DataType = matches[3]
		swaggerParameter.Required = strings.ToLower(matches[4]) == "true"
		swaggerParameter.Description = matches[5]

		operation.Parameters = append(operation.Parameters, swaggerParameter)
	}

	return nil
}

// @Accept  json
func (operation *Operation) ParseAcceptComment(commentLine string) error {
	accepts := strings.Split(strings.TrimSpace(strings.TrimSpace(commentLine[len("@Accept"):])), ",")
	for _, a := range accepts {
		switch a {
		case "json":
			operation.Consumes = append(operation.Consumes, ContentTypeJson)
			operation.Produces = append(operation.Produces, ContentTypeJson)
		case "xml":
			operation.Consumes = append(operation.Consumes, ContentTypeXml)
			operation.Produces = append(operation.Produces, ContentTypeXml)
		case "plain":
			operation.Consumes = append(operation.Consumes, ContentTypePlain)
			operation.Produces = append(operation.Produces, ContentTypePlain)
		case "html":
			operation.Consumes = append(operation.Consumes, ContentTypeHtml)
			operation.Produces = append(operation.Produces, ContentTypeHtml)
		}
	}
	return nil
}

// @router /customer/get-wishlist/{wishlist_id} [get]
func (operation *Operation) ParseRouterComment(commentLine string) error {
	sourceString := strings.TrimSpace(commentLine[len("@router"):])

	re := regexp.MustCompile(`([\w\.\/\-{}]+)[^\[]+\[([^\]]+)`)
	var matches []string

	if matches = re.FindStringSubmatch(sourceString); len(matches) != 3 {
		return fmt.Errorf("Can not parse router comment \"%s\", skipped.", commentLine)
	}

	operation.Path = matches[1]
	operation.HttpMethod = strings.ToUpper(matches[2])
	return nil
}

// @Success 200 {object} model.OrderRow "Error message, if code != 200"
func (operation *Operation) ParseResponseComment(commentLine string) error {
	re := regexp.MustCompile(`([\d]+)[\s]+([\w\{\}]+)[\s]+([\w\.\/]+)[^"]*(.*)?`)
	var matches []string

	if matches = re.FindStringSubmatch(commentLine); len(matches) != 5 {
		return fmt.Errorf("Can not parse response comment \"%s\", skipped.", commentLine)
	}

	response := ResponseMessage{}
	if code, err := strconv.Atoi(matches[1]); err != nil {
		return errors.New("Success http code must be int")
	} else {
		response.Code = code
	}

	if matches[2] == "{object}" || matches[2] == "{array}" {
		model := NewModel(operation.parser)
		response.ResponseModel = matches[3]
		if err, innerModels := model.ParseModel(response.ResponseModel, operation.parser.CurrentPackage); err != nil {
			return err
		} else {
			response.ResponseModel = model.Id
			if matches[1] == "{array}" {
				operation.SetItemsType(model.Id)
				operation.Type = "array"
			} else {
				operation.Type = model.Id
			}

			operation.models = append(operation.models, model)
			operation.models = append(operation.models, innerModels...)
		}
	}
	response.Message = strings.Trim(matches[4], "\"")

	operation.ResponseMessages = append(operation.ResponseMessages, response)
	return nil
}
