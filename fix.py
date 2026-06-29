import os, re

def replace_in_file(path, replacements):
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    for old, new in replacements:
        content = content.replace(old, new)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

replace_in_file('internal/api/handler.go', [
    ('c.Request().Context()()', 'c.Request().Context()'),
])

replace_in_file('internal/api/auth.go', [
    ('c.Request().Context()()', 'c.Request().Context()'),
    ('refreshTokenString, err :=', 'cookie, err :='),
    ('refreshTokenString.Value', 'cookie.Value'),
    ('jwt.Parse(refreshTokenString', 'jwt.Parse(cookie.Value'),
    ('cookie.Value = refresh', 'cookie.Value = newRefreshToken'),
])

# fixing auth.go
with open('internal/api/auth.go', 'r', encoding='utf-8') as f:
    content = f.read()

# fix logout unused refresh
content = re.sub(r'func \(h \*Handler\) Logout\(c echo\.Context\) error \{\n\tcookie := new\(http\.Cookie\)\n\tcookie\.Name = "refresh_token"\n\tcookie\.Value = newRefreshToken\n', r'func (h *Handler) Logout(c echo.Context) error {\n\tcookie := new(http.Cookie)\n\tcookie.Name = "refresh_token"\n\tcookie.Value = ""\n', content)

# remove redundant naked returns in handler.go
with open('internal/api/handler.go', 'r', encoding='utf-8') as f:
    hcontent = f.read()

hcontent = re.sub(r'\n\t\treturn\n\t}', r'\n\t}', hcontent)
hcontent = re.sub(r'\n\treturn\n}', r'\n}', hcontent)

with open('internal/api/handler.go', 'w', encoding='utf-8') as f:
    f.write(hcontent)

with open('internal/api/auth.go', 'w', encoding='utf-8') as f:
    f.write(content)
