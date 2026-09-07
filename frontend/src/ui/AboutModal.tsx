import { useEffect, useRef } from "react";

const AUTHOR_NAME = "Vilson de Freitas Pacheco";
const AUTHOR_EMAIL = "vilsonfreitaspacheco@gmail.com";
const REPO_URL = "https://github.com/vilsonfr/volcanopredict";

export function AboutModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);

  // <dialog> só entrega foco preso, ESC e backdrop nativos quando aberto via
  // showModal(); alternar o atributo `open` no JSX não faz isso.
  useEffect(() => {
    const el = dialogRef.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  return (
    <dialog ref={dialogRef} className="about" onClose={onClose}>
      <div className="about__head">
        <h2>Sobre o VolcanoPredict</h2>
        <button
          type="button"
          className="about__close"
          onClick={onClose}
          aria-label="Fechar"
        >
          ×
        </button>
      </div>

      <p>
        Plataforma científica aberta para monitoramento, análise e detecção de
        anomalias em atividade vulcânica.
      </p>

      <p className="about__warning">
        O sistema <strong>não prevê erupções</strong>. Ele reconstrói o
        histórico e o estado quase em tempo real dos vulcões e identifica sinais
        estatisticamente anormais, sempre com incerteza explícita. Nenhum
        resultado aqui constitui alerta oficial de emergência — consulte sempre
        as autoridades competentes.
      </p>

      <dl className="about__meta">
        <dt>Autor</dt>
        <dd>{AUTHOR_NAME}</dd>

        <dt>Contato</dt>
        <dd>
          <a href={`mailto:${AUTHOR_EMAIL}`}>{AUTHOR_EMAIL}</a>
        </dd>

        <dt>Projeto</dt>
        <dd>
          <a href={REPO_URL} target="_blank" rel="noreferrer noopener">
            Aberto e público
          </a>
        </dd>
      </dl>

      <h3>Fontes de dados</h3>
      <ul className="about__sources">
        <li>
          Catálogo de vulcões:{" "}
          <a
            href="https://volcano.si.edu/"
            target="_blank"
            rel="noreferrer noopener"
          >
            Global Volcanism Program, Smithsonian Institution
          </a>{" "}
          — Volcanoes of the World
        </li>
        <li>
          Imagem de satélite: Esri, Maxar, Earthstar Geographics e a comunidade
          de usuários GIS
        </li>
      </ul>
    </dialog>
  );
}
