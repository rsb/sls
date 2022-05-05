// Package pstore implements a parameter store client used specifically
// to managing configuration data for microservices
package pstore

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/rsb/failure"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type AdapterAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	GetParameters(ctx context.Context, params *ssm.GetParametersInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersOutput, error)
	GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
	DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

type Client struct {
	api         AdapterAPI
	isEncrypted bool
}

// NewClient simple constructor to inject private api and configuration values
func NewClient(api AdapterAPI, isEncrypted bool) (*Client, error) {
	if api == nil {
		return nil, failure.System("api is nil, an initialized ssmiface.SSMAPI is required")
	}

	return &Client{api: api, isEncrypted: isEncrypted}, nil
}

func (c *Client) IsEncrypted() bool {
	return c.isEncrypted
}

// Param will retrieve a single parameter as `key` returning the value always as a string.
// If the parameter does not exist a NotFound error is returned
func (c *Client) Param(ctx context.Context, key string) (string, error) {
	var result string
	if key == "" {
		return result, failure.System("key is empty, a non empty key is required")
	}
	in := ssm.GetParameterInput{
		Name:           aws.String(key),
		WithDecryption: c.IsEncrypted(),
	}

	out, err := c.api.GetParameter(ctx, &in)
	if err != nil {
		// handleAPIError separates out the NotFound
		return result, handleAPIError(err, "c.api.GetParameter failed (%s)", key)
	}

	if out != nil && out.Parameter != nil && out.Parameter.Value != nil {
		result = *out.Parameter.Value
	}

	return result, nil
}

// Path retrieves one or more params in a specific hierarchy, controlled by path.
func (c *Client) Path(ctx context.Context, path string, recursive ...bool) (map[string]string, error) {
	result := map[string]string{}
	isRecursive := true

	if path == "" {
		return result, failure.System("path is empty")
	}

	path = c.EnsurePathPrefix(path)

	if len(recursive) > 0 && recursive[0] == false {
		isRecursive = false
	}
	in := ssm.GetParametersByPathInput{
		Path:           aws.String(path),
		WithDecryption: c.IsEncrypted(),
		Recursive:      isRecursive,
	}

	pager := ssm.NewGetParametersByPathPaginator(c.api, &in)

	var errs []error
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			errs = append(errs, failure.Wrap(err, "pager.NextPage failed"))
		}
		for _, p := range out.Parameters {
			if p.Name == nil || p.Value == nil {
				continue
			}
			result[*p.Name] = *p.Value
		}
	}

	return result, nil
}

// Collect retrieves one or many params regardless of hierarchy.
// Note: a second array of strings will report on any invalid params that were sent
func (c *Client) Collect(ctx context.Context, keys ...string) (map[string]string, []string, error) {
	if len(keys) == 0 {
		return nil, nil, failure.System("keys must have at least one key")
	}

	var names []string

	for _, k := range keys {
		names = append(names, k)
	}

	in := ssm.GetParametersInput{
		Names:          names,
		WithDecryption: c.IsEncrypted(),
	}

	out, err := c.api.GetParameters(ctx, &in)
	if err != nil {
		return nil, nil, failure.ToSystem(err, "c.api.GetParametersWithContext failed (%v)", keys)
	}

	var invalid []string
	result := map[string]string{}
	for _, p := range out.Parameters {
		if p.Name == nil || p.Value == nil {
			continue
		}
		result[*p.Name] = *p.Value
	}

	for _, i := range out.InvalidParameters {
		if i == "" {
			continue
		}

		invalid = append(invalid, i)
	}

	return result, invalid, nil
}

// Delete will remove a single param from the store and return its old value.
// If the parameter does not exist a NotFound error is returned
func (c *Client) Delete(ctx context.Context, key string) (string, error) {
	var result string

	result, err := c.Param(ctx, key)
	if err != nil {
		return result, failure.Wrap(err, "c.Param failed (%s)", key)
	}

	in := ssm.DeleteParameterInput{
		Name: aws.String(key),
	}

	if _, err = c.api.DeleteParameter(ctx, &in); err != nil {
		return result, failure.ToSystem(err, "c.api.DeleteParameter failed (%s)", key)
	}

	return result, nil
}

// Put will check the existence of the parameter and only change them if they are
// different, or it does not exist
func (c *Client) Put(ctx context.Context, key, value string, overwrite ...bool) (string, error) {
	old, err := c.Param(ctx, key)
	if err != nil && !failure.IsNotFound(err) {
		return old, failure.Wrap(err, "c.Param failed")
	}

	// if we found something and the values are the same, then nothing to do
	if !failure.IsNotFound(err) && old == value {
		return old, nil
	}

	var isOverwrite bool
	if len(overwrite) > 0 && overwrite[0] == true {
		isOverwrite = true
	}

	if isOverwrite == false && old != "" {
		return old, failure.System("param (%s) exists but overwrite is false", key)
	}

	in := ssm.PutParameterInput{
		Name:      aws.String(key),
		Type:      types.ParameterTypeString,
		Value:     aws.String(value),
		Overwrite: isOverwrite,
		Tier:      types.ParameterTierStandard,
	}

	if _, err := c.api.PutParameter(ctx, &in); err != nil {
		return old, failure.ToSystem(err, "c.api.PutParameterWithContext failed (%s)", key)
	}

	return old, nil
}

func (c *Client) EnsurePathPrefix(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return path
}

func handleAPIError(err error, msg string, a ...interface{}) error {
	if err == nil {
		return nil
	}

	result := failure.ToSystem(err, msg, a...)
	var pne *types.ParameterNotFound
	if errors.As(err, &pne) {
		result = failure.ToNotFound(err, msg, a...)
	}

	return result
}