-- +goose Up
-- Task 2.3 (ingestao-fontes spec, design.md D6). Esta tabela não é
-- telemetria descartável: é parte do modelo de dados.
--
-- A §74 proíbe interpretar ausência de dado como ausência de atividade.
-- Sem um registro do que foi coletado e quando, "não houve sismo nesta
-- janela" e "não houve coleta nesta janela" são indistinguíveis — as duas
-- se parecem com zero linhas em `earthquakes`. A cobertura é derivada
-- daqui, nunca inferida da tabela de dados.

CREATE TABLE ingestion_runs (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL REFERENCES data_sources(id),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- NULL enquanto a execução está em andamento. É assim que uma
    -- execução interrompida por queda do processo se distingue de uma que
    -- terminou: ela fica para sempre sem finished_at, com resultado
    -- 'running'.
    finished_at TIMESTAMPTZ,
    -- A janela consultada na fonte, em tempo da FONTE (ocorrência ou
    -- atualização, conforme o modo). Distinta de started_at/finished_at,
    -- que são tempo nosso.
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    -- 'incremental' (ancorada na última execução bem-sucedida) ou
    -- 'backfill'/'manual' (janela explícita). Muda como a âncora da
    -- próxima execução é calculada.
    mode TEXT NOT NULL,
    result TEXT NOT NULL,
    inserted INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    unchanged INTEGER NOT NULL DEFAULT 0,
    rejected INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    CONSTRAINT ingestion_runs_result_known
        CHECK (result IN ('running', 'success', 'failure', 'skipped')),
    CONSTRAINT ingestion_runs_window_ordered
        CHECK (window_start <= window_end),
    -- Falha sem causa registrada seria um registro que não explica nada.
    CONSTRAINT ingestion_runs_failure_requires_message
        CHECK (result <> 'failure' OR (error_message IS NOT NULL AND error_message <> '')),
    -- Execução terminada tem de ter fim; execução em andamento, não.
    CONSTRAINT ingestion_runs_finished_iff_terminal
        CHECK ((result = 'running') = (finished_at IS NULL))
);

-- Sustenta as duas perguntas que esta tabela existe para responder: "qual
-- foi a última execução desta fonte?" e "esta janela foi coberta?".
CREATE INDEX ingestion_runs_source_started_idx
    ON ingestion_runs (source_id, started_at DESC);
CREATE INDEX ingestion_runs_source_window_idx
    ON ingestion_runs (source_id, window_start, window_end)
    WHERE result = 'success';

-- +goose Down
DROP TABLE IF EXISTS ingestion_runs;
