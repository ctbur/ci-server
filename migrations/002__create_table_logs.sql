CREATE TABLE logs (
    id BIGSERIAL PRIMARY KEY,

    build_id BIGINT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    text TEXT NOT NULL,

    CONSTRAINT fk_build
        FOREIGN KEY (build_id)
        REFERENCES builds (id)
        ON DELETE CASCADE
);
