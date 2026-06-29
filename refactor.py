import os
import re

def replace_in_file(path, replacements):
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    for old, new in replacements:
        content = re.sub(old, new, content)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

# Refactor internal/api/handler.go
replace_in_file('internal/api/handler.go', [
    (r'"github\.com/gin-gonic/gin"', r'"github.com/labstack/echo/v4"'),
    (r'func \(h \*Handler\) ([a-zA-Z0-9_]+)\(c \*gin\.Context\)', r'func (h *Handler) \1(c echo.Context) error'),
    (r'c\.ShouldBindJSON\(&([a-zA-Z0-9_]+)\)', r'c.Bind(&\1)'),
    (r'c\.JSON\((http\.[a-zA-Z0-9_]+), gin\.H', r'return c.JSON(\1, echo.Map'),
    (r'c\.JSON\((http\.[a-zA-Z0-9_]+), ([a-zA-Z0-9_&]+)\)', r'return c.JSON(\1, \2)'),
    (r'limitStr := c\.DefaultQuery\("limit", "20"\)', r'limitStr := c.QueryParam("limit")\n\tif limitStr == "" {\n\t\tlimitStr = "20"\n\t}'),
    (r'c\.Query\("cursor"\)', r'c.QueryParam("cursor")'),
    (r'c\.Query\("type"\)', r'c.QueryParam("type")'),
    (r'c\.Param\("id"\)', r'c.Param("id")'),
    (r'return c\.JSON(.*?)\n\t\treturn', r'return c.JSON\1'), # remove redundant return
    (r'return c\.JSON(.*?)\n\treturn', r'return c.JSON\1'),
])

def cleanup_handler_returns(path):
    with open(path, 'r', encoding='utf-8') as f:
        lines = f.readlines()
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        if line.strip() == "return" and i > 0 and "return c.JSON" in lines[i-1]:
            i += 1
            continue
        out.append(line)
        i += 1
    content = "".join(out)
    
    # Add return nil to end of functions
    content = re.sub(r'}\n\n// ([a-zA-Z0-9_]+)', r'\treturn nil\n}\n\n// \1', content)
    # the last function Stats() ends with }
    if content.endswith('}'):
        pass # Will manually check if it needs return nil
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

cleanup_handler_returns('internal/api/handler.go')

# Refactor internal/api/auth.go
replace_in_file('internal/api/auth.go', [
    (r'"github\.com/gin-gonic/gin"', r'"github.com/labstack/echo/v4"'),
    (r'func \(h \*Handler\) ([a-zA-Z0-9_]+)\(c \*gin\.Context\)', r'func (h *Handler) \1(c echo.Context) error'),
    (r'c\.ShouldBindJSON\(&([a-zA-Z0-9_]+)\)', r'c.Bind(&\1)'),
    (r'c\.JSON\((http\.[a-zA-Z0-9_]+), gin\.H', r'return c.JSON(\1, echo.Map'),
    (r'c\.JSON\((http\.[a-zA-Z0-9_]+), ([a-zA-Z0-9_&]+)\)', r'return c.JSON(\1, \2)'),
])

def fix_auth_cookie():
    with open('internal/api/auth.go', 'r', encoding='utf-8') as f:
        content = f.read()
    # Replace cookie logic
    content = content.replace('tokenStr, err := c.Cookie("refresh_token")', 'cookie, err := c.Cookie("refresh_token")\n\tif err != nil {\n\t\treturn c.JSON(http.StatusUnauthorized, echo.Map{"error": "missing refresh token"})\n\t}\n\ttokenStr := cookie.Value')
    
    content = re.sub(r'return c\.JSON(.*?)\n\t\treturn', r'return c.JSON\1', content)
    content = re.sub(r'return c\.JSON(.*?)\n\treturn', r'return c.JSON\1', content)
    content = re.sub(r'}\n\n// ([a-zA-Z0-9_]+)', r'\treturn nil\n}\n\n// \1', content)
    
    # In auth.go there's a setting cookie: c.SetCookie(...)
    # Echo uses c.SetCookie(cookie)
    cookie_repl = '''cookie := new(http.Cookie)
	cookie.Name = "refresh_token"
	cookie.Value = refresh
	cookie.MaxAge = 7 * 24 * 3600
	cookie.Path = "/"
	cookie.Domain = ""
	cookie.Secure = false
	cookie.HttpOnly = true
	c.SetCookie(cookie)'''
    content = re.sub(r'c\.SetCookie\("refresh_token".*?\)', cookie_repl, content)

    with open('internal/api/auth.go', 'w', encoding='utf-8') as f:
        f.write(content)

fix_auth_cookie()

# Refactor internal/api/middleware.go
replace_in_file('internal/api/middleware.go', [
    (r'"github\.com/gin-gonic/gin"', r'"github.com/labstack/echo/v4"'),
    (r'gin\.HandlerFunc', r'echo.MiddlewareFunc'),
    (r'func\(c \*gin\.Context\)', r'func(next echo.HandlerFunc) echo.HandlerFunc {\n\t\treturn func(c echo.Context) error'),
    (r'c\.AbortWithStatusJSON\((http\.[a-zA-Z0-9_]+), gin\.H', r'return c.JSON(\1, echo.Map'),
    (r'c\.Next\(\)', r'return next(c)'),
    (r'c\.ClientIP\(\)', r'c.RealIP()'),
    (r'c\.Request\.Method', r'c.Request().Method'),
    (r'c\.Request\.URL\.Path', r'c.Request().URL.Path'),
    (r'c\.Writer\.Status\(\)', r'c.Response().Status'),
    (r'c\.Header\(', r'c.Response().Header().Set('),
])

def fix_middleware_ends():
    with open('internal/api/middleware.go', 'r', encoding='utf-8') as f:
        content = f.read()
    content = re.sub(r'\n}\n\n//', r'\n\t}\n}\n\n//', content)
    content = re.sub(r'\n}\n$', r'\n\t}\n}\n', content)
    with open('internal/api/middleware.go', 'w', encoding='utf-8') as f:
        f.write(content)
fix_middleware_ends()

# Refactor internal/api/router.go
replace_in_file('internal/api/router.go', [
    (r'"github\.com/gin-gonic/gin"', r'"github.com/labstack/echo/v4"'),
    (r'\*gin\.Engine', r'*echo.Echo'),
    (r'gin\.SetMode\(gin\.ReleaseMode\)\n', r''),
    (r'r := gin\.New\(\)', r'e := echo.New()\n\te.HideBanner = true\n\te.HidePort = true'),
    (r'r\.Use\(gin\.Recovery\(\)\)\n', r''),
    (r'r\.Use\(', r'e.Use('),
    (r'r\.GET\(', r'e.GET('),
    (r'gin\.WrapH\(promhttp\.Handler\(\)\)', r'echo.WrapHandler(promhttp.Handler())'),
    (r'api := r\.Group', r'api := e.Group'),
    (r'auth := r\.Group', r'auth := e.Group'),
    (r'return r', r'return e'),
])

# Refactor cmd/server/main.go
replace_in_file('cmd/server/main.go', [
    (r'r\.Run\(":" \+ cfg\.ServerPort\)', r'r.Start(":" + cfg.ServerPort)'),
])

print("done")
