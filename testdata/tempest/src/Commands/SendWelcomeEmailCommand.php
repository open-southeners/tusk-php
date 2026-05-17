<?php

declare(strict_types=1);

namespace App\Commands;

use Tempest\Console\Console;
use Tempest\Console\ConsoleCommand;
use Tempest\Console\HasConsole;
use App\Services\MailService;
use App\Services\UserService;

final class SendWelcomeEmailCommand {
    use HasConsole;

    public function __construct(
        private readonly UserService $userService,
        private readonly MailService $mailService,
    ) {}

    #[ConsoleCommand(
        name: 'app:send-welcome-email',
        description: 'Send a welcome email to a specific user',
    )]
    public function __invoke(int $userId, bool $force = false): void
    {
        $user = $this->userService->find($userId);

        if ($user === null) {
            $this->console->error("User {$userId} not found.");
            return;
        }

        if (!$force && $this->console->confirm("Send welcome email to {$user->name}?")) {
            $this->console->info("Aborted.");
            return;
        }

        $this->mailService->sendWelcome($user);

        $this->console->success("Welcome email sent to {$user->name}.");
    }
}
