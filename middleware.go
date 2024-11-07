package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/patrickmn/go-cache"
)

// validateToken verifica se o token é admin ou usuário e retorna o tipo e dados do usuário
func (s *server) validateToken(token string) (bool, Values, error) {
	if token == "" {
		return false, Values{}, errors.New("no token provided")
	}

	// Primeiro verifica se é token admin
	if token == *adminToken {
		return true, Values{}, nil
	}

	// Se não for admin, busca informações do usuário
	var userid = 0
	var txtid = ""
	var webhook = ""
	var jid = ""
	var events = ""

	// Verifica cache primeiro
	myuserinfo, found := userinfocache.Get(token)
	if found {
		return false, myuserinfo.(Values), nil
	}

	// Busca no banco de dados
	rows, err := s.db.Query("SELECT id,webhook,jid,events FROM users WHERE token=$1 LIMIT 1", token)
	if err != nil {
		return false, Values{}, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&txtid, &webhook, &jid, &events)
		if err != nil {
			return false, Values{}, err
		}
		userid, _ = strconv.Atoi(txtid)
		v := Values{map[string]string{
			"Id":      txtid,
			"Jid":     jid,
			"Webhook": webhook,
			"Token":   token,
			"Events":  events,
		}}
		userinfocache.Set(token, v, cache.NoExpiration)
		return false, v, nil
	}

	return false, Values{}, errors.New("invalid token")
}

// Middleware unificado para autenticação
func (s *server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Obtém o token do header
		token := r.Header.Get("token")
		if token == "" {
			token = strings.Join(r.URL.Query()["token"], "")
		}

		// Valida o token
		isAdmin, userInfo, err := s.validateToken(token)
		if err != nil {
			s.Respond(w, r, http.StatusUnauthorized, errors.New("Unauthorized"))
			return
		}

		// Para rotas admin, verifica se é token admin
		if strings.HasPrefix(r.URL.Path, "/admin") && !isAdmin {
			s.Respond(w, r, http.StatusUnauthorized, errors.New("Unauthorized"))
			return
		}

		// Para rotas de usuário, adiciona informações ao contexto
		if !isAdmin {
			ctx := context.WithValue(r.Context(), "userinfo", userInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Se chegou aqui é admin
		next.ServeHTTP(w, r)
	})
}
