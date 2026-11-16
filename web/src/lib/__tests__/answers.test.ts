import type { Question } from '../../api/types';
import { answerSummary, storedToValues } from '../answers';

const qs: Question[] = [
  { id: 'whatsapp', text: 'WhatsApp?', hint: '', kind: 'phone', options: null },
  { id: 'fotos', text: 'Fotos?', hint: '', kind: 'choice', options: [{ value: 'sim', label: 'Sim, vou enviar' }, { value: 'nao', label: 'Não' }] },
  {
    id: 'servicos',
    text: 'Serviços?',
    hint: '',
    kind: 'multi',
    options: [
      { value: 'trancas', label: 'Tranças' },
      { value: 'cor', label: 'Cor, corte e brushing' },
      { value: 'unhas', label: 'Unhas' },
    ],
  },
  { id: 'entrega', text: 'Entrega?', hint: '', kind: 'text', options: null },
];

// What product.labelAnswers stores for these answers.
const stored = {
  whatsapp: '+258841234567',
  fotos: 'Sim, vou enviar',
  servicos: 'Tranças, Cor, corte e brushing',
  entrega: '50 MT em Maputo',
  _correction: 'Também vendemos a revendedores.',
};

describe('storedToValues', () => {
  it('turns stored labels back into form values', () => {
    expect(storedToValues(qs, stored)).toEqual({
      whatsapp: '841234567',
      fotos: 'sim',
      servicos: 'trancas,cor',
      entrega: '50 MT em Maputo',
    });
  });
  it('drops answers that no longer match an option', () => {
    expect(storedToValues(qs, { fotos: 'Talvez' })).toEqual({});
    expect(storedToValues(qs, undefined)).toEqual({});
  });
});

describe('answerSummary', () => {
  it('keeps question order and leaves the correction out', () => {
    expect(answerSummary(qs, stored)).toEqual(['+258841234567', 'Sim, vou enviar', 'Tranças, Cor, corte e brushing', '50 MT em Maputo']);
    expect(answerSummary(undefined, { b: '2', _correction: 'x', a: '1' })).toEqual(['2', '1']);
  });
});
