package handler

import (
	"errors"
	"fmt"

	"github.com/dgrijalva/jwt-go"
	"github.com/turnerlabs/udeploy/component/auth"

	"github.com/turnerlabs/udeploy/component/cfg"
	"github.com/turnerlabs/udeploy/component/integration/oauth"

	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	echosession "github.com/turnerlabs/udeploy/component/session"
)

// Logout ...
func Logout(c echo.Context) error {
	store := echosession.FromContext(c)
	store.Delete(auth.AuthSessionName)

	if err := store.Save(); err != nil {
		return err
	}

	return c.Redirect(http.StatusTemporaryRedirect, cfg.Get["OAUTH_SIGN_OUT_URL"])
}

// Login ...
func Login(c echo.Context) error {
	state, err := json.Marshal(oauth.UpdateState(c.QueryParam(auth.UserURLParam)))
	if err != nil {
		return err
	}

	url := oauth.Config.AuthCodeURL(string(state))

	return c.Redirect(http.StatusTemporaryRedirect, url)
}

// Response ...
func Response(c echo.Context) error {

	returnedState := oauth.State{}
	if err := json.Unmarshal([]byte(c.QueryParam("state")), &returnedState); err != nil {
		return err
	}

	if returnedState.Invalid() {
		return errors.New("invalid state")
	}

	token, err := oauth.Config.Exchange(context.Background(), c.QueryParam("code"))
	if err != nil {
		return err
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return fmt.Errorf("id_token missing or malformed")
	}

	// Currently this function causes an error that is ignored since the public key is not
	// defined. Parsing the token does not require the JWT signature verification. At
	// some point it may be worth verifying the signer.
	//
	// https://docs.microsoft.com/en-us/azure/active-directory/develop/active-directory-signing-key-rollover
	IDToken, _ := jwt.Parse(rawIDToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return jwt.ParseRSAPublicKeyFromPEM([]byte{})
	})

	claims, ok := IDToken.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("failed to parse id_token JWT claims")
	}

	uid, ok := claims["email"].(string)
	if !ok {
		return fmt.Errorf("'email' claim missing from id_token")
	}

	store := echosession.FromContext(c)
	store.Set(auth.AuthSessionName, token)
	store.Set(auth.UserIDParam, uid)

	if err := store.Save(); err != nil {
		return err
	}

	return c.Redirect(http.StatusTemporaryRedirect, returnedState.UserRequestedPath)
}
