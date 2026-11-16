import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Job, Question } from '../../../api';
import { Questions } from '../Questions';
import { Progress } from '../Progress';

const questions: Question[] = [
  { id: 'whatsapp', text: 'Qual é o WhatsApp?', hint: '', kind: 'phone', options: null },
  {
    id: 'fotos',
    text: 'Tens fotos?',
    hint: '',
    kind: 'choice',
    options: [
      { value: 'sim', label: 'Sim' },
      { value: 'nao', label: 'Não' },
    ],
  },
  { id: 'entrega', text: 'Quanto custa a entrega?', hint: '', kind: 'text', options: null },
];

describe('Questions', () => {
  it('walks the questions, validates the phone and sends values', async () => {
    const user = userEvent.setup();
    const onDone = vi.fn();
    render(<Questions questions={questions} onDone={onDone} onSkipAll={() => undefined} />);

    expect(screen.getByText('Pergunta 1 de 3')).toBeInTheDocument();
    await user.type(screen.getByPlaceholderText('84 123 4567'), '821234567');
    await user.click(screen.getByRole('button', { name: 'Continuar' }));
    expect(screen.getByRole('alert')).toHaveTextContent('M-Pesa (84, 85) ou e-Mola (86, 87)');

    await user.clear(screen.getByPlaceholderText('84 123 4567'));
    await user.type(screen.getByPlaceholderText('84 123 4567'), '84 123 4567');
    await user.click(screen.getByRole('button', { name: 'Continuar' }));

    expect(screen.getByText('Pergunta 2 de 3')).toBeInTheDocument();
    await user.keyboard('2');
    expect(screen.getByRole('radio', { name: /Não/ })).toHaveAttribute('aria-checked', 'true');
    await user.click(screen.getByRole('button', { name: 'Continuar' }));

    await user.click(screen.getByRole('button', { name: 'Saltar' }));
    expect(onDone).toHaveBeenCalledWith({ whatsapp: '841234567', fotos: 'nao' }, '');
  });

  it('skips everything on request', async () => {
    const user = userEvent.setup();
    const onSkipAll = vi.fn();
    render(<Questions questions={questions} onDone={() => undefined} onSkipAll={onSkipAll} />);
    await user.click(screen.getByRole('button', { name: 'Prefiro saltar' }));
    expect(onSkipAll).toHaveBeenCalledWith('');
  });

  it('shows what it understood and sends the correction', async () => {
    const user = userEvent.setup();
    const onDone = vi.fn();
    render(<Questions questions={questions.slice(1, 2)} understood="Percebi que é um site para a Mel da Ana." onDone={onDone} onSkipAll={() => undefined} />);
    expect(screen.getByText('O que percebi')).toBeInTheDocument();
    expect(screen.getByText('Percebi que é um site para a Mel da Ana.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Corrigir' }));
    await user.type(screen.getByLabelText(/O que está errado/), 'Vendemos também a revendedores.');
    await user.click(screen.getByRole('radio', { name: /Sim/ }));
    await user.click(screen.getByRole('button', { name: 'Construir site' }));
    expect(onDone).toHaveBeenCalledWith({ fotos: 'sim' }, 'Vendemos também a revendedores.');
  });

  it('offers to retry with the answers kept from a failed build', async () => {
    const user = userEvent.setup();
    const onDone = vi.fn();
    render(
      <Questions
        questions={questions}
        understood="Percebi."
        initial={{ whatsapp: '841234567', fotos: 'sim' }}
        initialCorrection="Só Maputo."
        onDone={onDone}
        onSkipAll={() => undefined}
      />,
    );
    expect(screen.getByText(/Guardei as tuas respostas/)).toBeInTheDocument();
    expect(screen.getByLabelText(/O que está errado/)).toHaveValue('Só Maputo.');
    await user.click(screen.getByRole('button', { name: 'Construir com estas respostas' }));
    expect(onDone).toHaveBeenCalledWith({ whatsapp: '841234567', fotos: 'sim' }, 'Só Maputo.');
  });
});

const job = (progress: string | undefined, status: Job['status'] = 'running'): Job => ({
  id: 1,
  kind: 'build',
  status,
  attempts: 1,
  progress,
  created_at: '',
  updated_at: '',
});

describe('Progress', () => {
  it('maps pipeline stages to steps and never goes back', () => {
    const { rerender } = render(<Progress job={job('content')} kind="build" />);
    expect(screen.getByText('0/4')).toBeInTheDocument();
    rerender(<Progress job={job('build')} kind="build" />);
    expect(screen.getByText('2/4')).toBeInTheDocument();
    rerender(<Progress job={job('content')} kind="build" />);
    expect(screen.getByText('2/4')).toBeInTheDocument();
    rerender(<Progress job={job('verify', 'succeeded')} kind="build" />);
    expect(screen.getByText('4/4')).toBeInTheDocument();
    expect(screen.getByText('Concluído')).toBeInTheDocument();
  });

  it('ignores the check an edit runs before changing the page', () => {
    const { rerender } = render(<Progress job={{ ...job('verify'), kind: 'edit' }} kind="edit" />);
    expect(screen.getByText('0/2')).toBeInTheDocument();
    rerender(<Progress job={{ ...job('edit'), kind: 'edit' }} kind="edit" />);
    rerender(<Progress job={{ ...job('verify'), kind: 'edit' }} kind="edit" />);
    expect(screen.getByText('1/2')).toBeInTheDocument();
  });
});
