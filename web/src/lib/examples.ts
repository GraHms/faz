// Example businesses from the prototype. The landing page offers them as
// prompts to start from; the mock API uses them to draw a believable page.
// None of them is a real customer.

export const businessTypes = ['Restaurante', 'Loja', 'Salão', 'Serviços', 'Freelancer', 'Agência'] as const;
export type BusinessType = (typeof businessTypes)[number];

export interface Example {
  name: string;
  type: BusinessType;
  color: string;
  prompt: string;
}

export const examples: Example[] = [
  {
    name: 'Mar Azul',
    type: 'Restaurante',
    color: '#0F5A5C',
    prompt:
      'Um site para a minha marisqueira Mar Azul na Costa do Sol, com menu e preços em meticais, fotos dos pratos e reservas pelo WhatsApp.',
  },
  {
    name: 'Nyeleti',
    type: 'Salão',
    color: '#7A2E4A',
    prompt: 'Site para o Salão Nyeleti na Matola: serviços, preços, galeria de tranças e marcações pelo WhatsApp.',
  },
  {
    name: 'Capulana & Co',
    type: 'Loja',
    color: '#B4452A',
    prompt:
      'Loja online de capulanas e roupa feita à medida, com catálogo, encomendas por WhatsApp e entregas em Maputo.',
  },
];

export interface SampleContent {
  tag: string;
  headline: string;
  lead: string;
  list: string;
  cta: string;
  contact: string;
  hours: string;
  address: string;
  items: [name: string, note: string, price: string][];
}

const restaurant: SampleContent = {
  tag: 'Marisqueira · Costa do Sol',
  headline: 'Marisco fresco, à beira da Costa do Sol.',
  lead: 'Camarão, lulas e peixe do dia, grelhados na hora. Mesa com vista para a baía.',
  list: 'Menu',
  cta: 'Reservar mesa',
  contact: 'Reserva a tua mesa',
  hours: 'Todos os dias, das 11h às 23h',
  address: 'Av. Marginal 4120, Costa do Sol, Maputo',
  items: [
    ['Camarão grelhado', 'Com batata frita e salada', '1 250 MZN'],
    ['Matapa com caranguejo', 'Receita da casa, arroz de coco', '680 MZN'],
    ['Lulas à moçambicana', 'Piri-piri e limão', '890 MZN'],
    ['Peixe-pedra inteiro', 'Para duas pessoas', '1 100 MZN'],
  ],
};

const salon: SampleContent = {
  tag: 'Salão de beleza · Matola',
  headline: 'Tranças, cor e cuidado, sem pressa.',
  lead: 'Marca pelo WhatsApp e escolhe o teu horário. Produtos profissionais e equipa de cinco cabeleireiras.',
  list: 'Serviços',
  cta: 'Marcar horário',
  contact: 'Marca o teu horário',
  hours: 'Terça a sábado, das 8h às 18h',
  address: 'Av. União Africana, Matola',
  items: [
    ['Box braids', '3 a 4 horas', '1 800 MZN'],
    ['Manicure gel', '45 minutos', '650 MZN'],
    ['Coloração', 'Inclui tratamento', '1 400 MZN'],
    ['Penteado para evento', 'Com marcação', '950 MZN'],
  ],
};

const shop: SampleContent = {
  tag: 'Loja online · Maputo',
  headline: 'Capulanas escolhidas a dedo, entregues em Maputo.',
  lead: 'Tecidos novos todas as semanas. Encomenda pelo WhatsApp e paga com M-Pesa na entrega.',
  list: 'Catálogo',
  cta: 'Encomendar',
  contact: 'Faz a tua encomenda',
  hours: 'Segunda a sábado, das 9h às 19h',
  address: 'Entregas em Maputo e Matola',
  items: [
    ['Capulana clássica', '2 panos', '450 MZN'],
    ['Vestido à medida', 'Pronto em 7 dias', '2 200 MZN'],
    ['Lenço de cabeça', 'Várias cores', '250 MZN'],
    ['Camisa de capulana', 'Tamanhos S a XXL', '1 350 MZN'],
  ],
};

const services: SampleContent = {
  tag: 'Estúdio · Maputo',
  headline: 'Marcas e sites para negócios moçambicanos.',
  lead: 'Conta-nos o que precisas e respondemos no mesmo dia pelo WhatsApp.',
  list: 'Serviços',
  cta: 'Pedir orçamento',
  contact: 'Fala connosco',
  hours: 'Segunda a sexta, das 8h às 17h',
  address: 'Sommerschield, Maputo',
  items: [
    ['Conversa inicial', '1 hora', 'Grátis'],
    ['Identidade visual', 'Entrega em 10 dias', '18 000 MZN'],
    ['Site com domínio', 'Entrega em 5 dias', '8 500 MZN'],
    ['Manutenção mensal', 'Sem fidelização', '3 200 MZN'],
  ],
};

export function sampleContent(type: string): SampleContent {
  switch (type) {
    case 'Restaurante':
      return restaurant;
    case 'Salão':
      return salon;
    case 'Loja':
      return shop;
    default:
      return services;
  }
}
