-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (
    token,
    updated_at,
    user_id,
    expires_at,
    revoked_at
) VALUES (
    $1, NOW(), $2, $3, NULL
);

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = NOW(), updated_at = NOW()
WHERE token = $1;

-- name: GetUserFromRefreshToken :one
SELECT * FROM refresh_tokens AS rt
INNER JOIN users AS u ON rt.user_id = u.id
WHERE rt.token = $1;