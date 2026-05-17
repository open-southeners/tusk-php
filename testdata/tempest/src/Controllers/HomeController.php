<?php

declare(strict_types=1);

namespace App\Controllers;

use Tempest\Http\Request;
use Tempest\Http\Response;
use Tempest\Http\Responses\Ok;
use Tempest\Http\Responses\Created;
use Tempest\Router\Get;
use Tempest\Router\Post;
use App\Models\User;
use App\Services\UserService;

final class HomeController {
    public function __construct(
        private readonly UserService $userService,
    ) {}

    #[Get('/')]
    public function index(): Response
    {
        $users = $this->userService->all();

        return new Ok(view: 'home.view.php', data: ['users' => $users]);
    }

    #[Get('/{id}')]
    public function show(int $id): Response
    {
        $user = $this->userService->find($id);

        if ($user === null) {
            return new \Tempest\Http\Responses\NotFound();
        }

        return new Ok(view: 'user.view.php', data: ['user' => $user]);
    }

    #[Post('/')]
    public function store(Request $request): Response
    {
        $user = $this->userService->create(
            name: $request->get('name'),
            email: $request->get('email'),
        );

        return new Created(view: 'user.view.php', data: ['user' => $user]);
    }
}
