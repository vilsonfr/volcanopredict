import type { Volcano } from "../api/client";

/**
 * Balão de detalhe ancorado a um vulcão no globo.
 *
 * A posição vem em pixels de tela, recalculada a cada quadro pelo Globe, de
 * modo que o balão acompanha o ponto quando a Terra gira. A seta na base é
 * o que liga visualmente o balão ao vulcão — sem ela, com 1.214 pontos
 * próximos, seria impossível saber a qual deles o painel se refere.
 */
export function VolcanoCallout({
  volcano,
  x,
  y,
  onClose,
}: {
  volcano: Volcano;
  x: number;
  y: number;
  onClose: () => void;
}) {
  const rows: Array<[string, string]> = [
    ["País", volcano.country || "—"],
    [
      "Elevação",
      volcano.elevation_m != null
        ? `${volcano.elevation_m.toLocaleString("pt-BR")} m`
        : "—",
    ],
    ["Evidência de atividade", volcano.status || "—"],
    [
      "Coordenadas",
      `${volcano.latitude.toFixed(4)}, ${volcano.longitude.toFixed(4)}`,
    ],
    ["Número GVP", volcano.source_ref],
  ];

  return (
    <div
      className="callout"
      style={{ left: `${x}px`, top: `${y}px` }}
      role="dialog"
      aria-label={`Detalhes de ${volcano.name}`}
    >
      <div className="callout__box">
        <div className="callout__head">
          <h3>{volcano.name}</h3>
          <button type="button" onClick={onClose} aria-label="Fechar">
            ×
          </button>
        </div>

        <dl className="callout__data">
          {rows.map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>

        {volcano.absent_from_source && (
          <p className="callout__absent">
            Este vulcão não consta mais no catálogo da fonte. O registro é
            mantido por rastreabilidade histórica.
          </p>
        )}

        <p className="callout__source">
          Dado observado do Global Volcanism Program, Smithsonian Institution.
          Nenhum valor aqui é derivado de modelo.
        </p>
      </div>
      <div className="callout__arrow" aria-hidden="true" />
    </div>
  );
}
